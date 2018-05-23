package reviewer

import (
        "net/http"
        "log"
        "io/ioutil"
        "fmt"
        "encoding/json"
        "sync"
        "sync/atomic"
        "bytes"
        "mime/multipart"
        "strconv"
        "regexp"
        "plugin"
        "errors"

        "rbbot/db"
        "rbplugindata/reviewdata"
)

var (
    config RbConfig
)

/**
 * A single file in ReviewBoard's diffs. Used to pick up links.
 */
type DiffFile struct {
    Id    int
    Links reviewdata.LinkContainer
}

/**
 * All of the files in a review.
 */
type DiffFileContainer struct {
    Files []DiffFile
}

/**
 * Ancillary data about a file that we pick up.
 */
type ReviewFileData struct {
    File struct {
        Dest_File string
    }
}

/**
 * The response from publishing a review. Used to pick up the ID.
 */
type ReviewResponse struct {
    Review struct {
        Id int
    }
}

/**
 * Wraps the review request that we receive.
 */
type ReviewContainer struct {
    Stat           string
    Review_Request reviewdata.ReviewRequest
}

/**
 * A ReviewerPlugin is something that provides the following functions.
 */
type ReviewerPlugin interface {
    Version()       (int,int,int) // The plguin's version (major minor micro)
    CanonicalName() string        // The plugin's canonical name
    Check(reviewdata.FileDiff,
          chan <- reviewdata.Comment,
          *sync.WaitGroup)           // Runs the review plugin on a file
    CheckReview(reviewdata.ReviewRequest,
                chan <- string,
                *sync.WaitGroup) // Runs the review plugin on the review request
}

/**
 * Retrieves diffed files for a review
 */
func GetDiffedFiles(link string) (error, DiffFileContainer) {
    req, err := http.NewRequest("GET", link + "/files/", nil)

    req.Header.Add("Authorization", config.RbToken)

    resp, err := (&http.Client{}).Do(req)

    if (err != nil) {
        log.Fatal(err)
    }
    defer resp.Body.Close()

    body, err := ioutil.ReadAll(resp.Body)

    if (err != nil) {
        log.Fatal(err)
    }

    var diffFiles DiffFileContainer

    if (json.Valid(body)) {
        err := json.Unmarshal(body, &diffFiles)

        if err != nil {
            log.Fatal(err)
        }
    } else {
        fmt.Printf("Invalid json\n")
    }

    return err, diffFiles
}

/**
 * Retrieves a single file diff from a review's file links.
 */
func GetFileDiff (links reviewdata.LinkContainer) (error, reviewdata.FileDiff) {
    req, err := http.NewRequest("GET", links.Self.Href, nil)

    req.Header.Add("Authorization", config.RbToken)
    req.Header.Add("Accept",
                   "application/vnd.reviewboard.org.diff.data+json")

    resp, err := (&http.Client{}).Do(req)

    if (err != nil) {
        log.Fatal(err)
    }
    defer resp.Body.Close()

    body, err := ioutil.ReadAll(resp.Body)

    if (err != nil) {
        log.Fatal(err)
    }

    var file reviewdata.FileDiff

    if (json.Valid(body)) {
        err := json.Unmarshal(body, &file)

        if err != nil {
            log.Fatal(err)
        }
    } else {
        fmt.Printf("Invalid json\n")
    }

    err, reviewFileData := GetFileData(links.Self.Href)

    if (err != nil) {
        log.Fatal(err)
    }

    err, entireFile := GetRawFile(links.Patched_File.Href)

    if ( err != nil) {
        log.Fatal(err)
    }

    file.Filename   = reviewFileData.File.Dest_File
    file.EntireFile = entireFile

    return err, file

}

/**
 * Retrieves a raw file from a review.
 */
func GetRawFile(link string) (error, []byte) {
    req, err := http.NewRequest("GET", link, nil)

    req.Header.Add("Authorization", config.RbToken)

    resp, err := (&http.Client{}).Do(req)

    if (err != nil) {
        log.Fatal(err)
    }
    defer resp.Body.Close()

    body, err := ioutil.ReadAll(resp.Body)

    if (err != nil) {
        log.Fatal(err)
    }

    return err, body
}

/**
 * Retrieves ancillary data about a file.
 */
func GetFileData(link string) (error, ReviewFileData) {
    req, err := http.NewRequest("GET", link, nil)

    req.Header.Add("Authorization", config.RbToken)

    resp, err := (&http.Client{}).Do(req)

    if (err != nil) {
        log.Fatal(err)
    }
    defer resp.Body.Close()

    body, err := ioutil.ReadAll(resp.Body)

    if (err != nil) {
        log.Fatal(err)
    }

    var rfd ReviewFileData

    err = json.Unmarshal(body, &rfd)

    if (err != nil) {
        log.Fatal(err)
    }

    return err, rfd
}

/**
 * Manages and collates comments.
 */
func ManageComments(inChan <- chan reviewdata.Comment,
                    outComments *reviewdata.CommentedFile,
                    wg          *sync.WaitGroup) {
    for {
        comment, ok := <- inChan

        if (!ok) {
            // The comment channel has closed, so no more comments are incoming.
            // We're done
            wg.Done()
            break
        }

        // TODO: HACK - set 0-length comments to 1
        if (comment.NumLines == 0) {
            comment.NumLines = 1
        }

        var added bool = false

        if _, exists := (*outComments).Comments[comment.Line]; exists {
            if (len((*outComments).Comments[comment.Line]) == 0) {
                (*outComments).Comments[comment.Line] =
                    append((*outComments).Comments[comment.Line], &comment)
                added = true
            } else {
                for i := 0;
                    i < len((*outComments).Comments[comment.Line]);
                    i++ {
                    if ((*outComments).Comments[comment.Line][i].NumLines ==
                        comment.NumLines) {
                        (*outComments).Comments[comment.Line][i].Text += "\n\n" +
                                                                    comment.Text
                        if (comment.RaiseIssue) {
                            (*outComments).Comments[comment.Line][i].
                                           RaiseIssue = true
                        }
                        added = true
                    }
                }

                if (!added) {
                    (*outComments).Comments[comment.Line] =
                        append((*outComments).Comments[comment.Line], &comment)
                }
            }
        } else {
            (*outComments).Comments[comment.Line] =
                append((*outComments).Comments[comment.Line], &comment)
        }
    }
}

/**
 * Runs all of the checkers on a single file, and collates comments.
 */
func CheckFileAndComment(file           reviewdata.FileDiff,
                         reviewIdStr    string,
                         responseIdStr  string,
                         commentCount  *int32,
                         wg            *sync.WaitGroup,
                         reviewPlugins []ReviewerPlugin) {
    // Count the plugins
    var numCheckers = len(reviewPlugins)

    // One comment manager
    var commentMgrWg sync.WaitGroup
    commentMgrWg.Add(1)

    var checkerGroup  sync.WaitGroup
    var commentedFile reviewdata.CommentedFile

    comments := make(chan reviewdata.Comment)

    commentedFile.FileId   = file.Id
    commentedFile.Comments = make(map[int][]*reviewdata.Comment)

    checkerGroup.Add(numCheckers)

    go ManageComments(comments, &commentedFile, &commentMgrWg)

    //Run the checkers
    for i := 0; i < numCheckers; i++ {
        go reviewPlugins[i].Check(file, comments, &checkerGroup)
    }

    // Wait for them all to complete
    checkerGroup.Wait()

    // Close the comment stream when all of the checkers are done. The comment
    // manager will then exit - wait for it to exit, so we're sure that all of
    // the comments have been processed
    close(comments)

    commentMgrWg.Wait()

    // If there are comments on the file, add them to the review
    if (len(commentedFile.Comments) > 0) {
        SendFileComments(reviewIdStr, responseIdStr, commentedFile)

        // Count the comments
        atomic.AddInt32(commentCount, int32(len(commentedFile.Comments)))
    }

    wg.Done()
}

/**
 * Runs all of the checker plugins, and submits comments to the review. Returns
 * the number of comments made, and a general review comment.
 */
func RunCheckersAndComment(reviewIdStr    string,
                           responseIdStr  string,
                           reviewRequest  reviewdata.ReviewRequest,
                           files         *[]reviewdata.FileDiff,
                           reviewPlugins  []ReviewerPlugin) (int, string) {
    var fileCheckWaitGroup   sync.WaitGroup
    var reviewCheckWaitGroup sync.WaitGroup
    var commentsMade int32 = 0

    fileCheckWaitGroup.Add(len(*files))
    reviewCheckWaitGroup.Add(len(reviewPlugins))

    var reviewCommentChan chan string = make(chan string, len(reviewPlugins))

    for i := 0; i < len(reviewPlugins); i++ {
        go reviewPlugins[i].CheckReview(reviewRequest,
                                        reviewCommentChan,
                                        &reviewCheckWaitGroup)
    }

    for i := 0; i < len(*files); i++ {
        go CheckFileAndComment((*files)[i],
                               reviewIdStr,
                               responseIdStr,
                               &commentsMade,
                               &fileCheckWaitGroup,
                               reviewPlugins)
    }

    // Wait for all file checks to complete
    fileCheckWaitGroup.Wait()
    // Wait for all review checks to complete
    reviewCheckWaitGroup.Wait()

    var generalComment string = ""

    for i := 0; i < len(reviewCommentChan); i++ {
        generalComment += <-reviewCommentChan + "\n"
    }

    return int(atomic.LoadInt32(&commentsMade)), generalComment
}

/**
 * Creates an empty review reply, to which comments can be attached.
 *
 * @param reviewId The review ID.
 *
 * @retval string The ID of the review reply, as a string.
 */
func CreateReviewReply (reviewId string) string {
    var b bytes.Buffer
    w := multipart.NewWriter(&b)

    fw, err := w.CreateFormField("body_top")

    if err != nil {
        log.Fatal(err)
    }

    if _, err := fw.Write([]byte("This is a test review")); err != nil {
        log.Fatal(err)
    }

    w.Close()

    var reviewUrl string = config.RbApiUrl + "/review-requests/" + reviewId + "/reviews/"

    // Post a new blank review, to which we will add comments
    req, err := http.NewRequest("POST", reviewUrl, &b)

    req.Header.Add("Authorization", config.RbToken)
    req.Header.Set("Content-Type",
                   w.FormDataContentType())

    resp, err := (&http.Client{}).Do(req)

    if (err != nil) {
        log.Fatal(err)
    }
    defer resp.Body.Close()

    body, _ := ioutil.ReadAll(resp.Body)

    var reviewResponse ReviewResponse

    if (json.Valid(body)) {
        err := json.Unmarshal(body, &reviewResponse)

        if err != nil {
            log.Fatal(err)
        }
    } else {
        fmt.Printf("Invalid json\n")
    }

    var reviewResponseIdString string = strconv.Itoa(reviewResponse.Review.Id)

    return reviewResponseIdString
}

/**
 * Sends all comments for a single file, adding them to an existing review
 * response.
 *
 * @param reviewId               The ID of the review being done.
 * @param reviewResponseIdString The ID of the existing review response.
 * @param comments               A CommentedFile containing all of the comments
 *                               for the file.
 */
func SendFileComments (reviewId               string,
                       reviewResponseIdString string,
                       comments               reviewdata.CommentedFile) {

    if (len(comments.Comments) > 0) {
        for line, commentList := range comments.Comments {
            for _, comment := range commentList {
                var commentBuffer bytes.Buffer
                commentWriter := multipart.NewWriter(&commentBuffer)

                cfw, err := commentWriter.CreateFormField("filediff_id")

                if err != nil {
                    log.Fatal(err)
                }

                var commentsId string = strconv.Itoa(comments.FileId)

                if _, err = cfw.Write([]byte(commentsId)); err != nil {
                    log.Fatal(err)
                }

                cfw, err = commentWriter.CreateFormField("first_line")

                if err != nil {
                    log.Fatal(err)
                }

                var commentLine string = strconv.Itoa(line)

                if _, err = cfw.Write([]byte(commentLine)); err != nil {
                    log.Fatal(err)
                }

                cfw, err = commentWriter.CreateFormField("num_lines")

                if err != nil {
                    log.Fatal(err)
                }

                if _, err = cfw.Write([]byte("1")); err != nil {
                    log.Fatal(err)
                }

                cfw, err = commentWriter.CreateFormField("text")

                if err != nil {
                    log.Fatal(err)
                }

                if _, err = cfw.Write([]byte(comment.Text)); err != nil {
                    log.Fatal(err)
                }

                cfw, err = commentWriter.CreateFormField("issue_opened")

                if err != nil {
                    log.Fatal(err)
                }

                var raiseIssue string

                if (comment.RaiseIssue) {
                    raiseIssue = "true"
                } else {
                    raiseIssue = "false"
                }

                if _, err = cfw.Write([]byte(raiseIssue)); err != nil {
                    log.Fatal(err)
                }

                cfw, err = commentWriter.CreateFormField("text")

                if err != nil {
                    log.Fatal(err)
                }

                if _, err = cfw.Write([]byte(comment.Text)); err != nil {
                    log.Fatal(err)
                }

                commentWriter.Close()

                var reviewCommentUrl string = "http://reviews.example.com/api/review-requests/" +
                                              reviewId +
                                              "/reviews/" +
                                              reviewResponseIdString +
                                              "/diff-comments/"

                // Post the comments
                req, err := http.NewRequest("POST",
                                            reviewCommentUrl,
                                            &commentBuffer)

                req.Header.Add("Authorization", config.RbToken)
                req.Header.Set("Content-Type",
                               commentWriter.FormDataContentType())

                resp, err := (&http.Client{}).Do(req)

                if (err != nil) {
                    log.Fatal(err)
                }
                defer resp.Body.Close()

                //TODO - error handling
            }
        }
    }
}

/**
 * Publishes a review response, making it public and unmodifiable.
 *
 * @param reviewId      The ID of the review whose response is being published.
 * @param responseIdStr The ID of the response being published.
 * @param requester     The name of the entity that requested the review.
 * @param commented     Whether any checkers made comments.
 * @param extraComment  A comment from any checkers which did not relate to
 *                      files.
 * @param seenBefore    Whether we've seen this review before.
 */
func PublishReview(reviewId      string,
                   responseIdStr string,
                   requester     string,
                   commented     bool,
                   extraComment  string,
                   seenBefore    bool) {

    var publishBuffer bytes.Buffer
    publishWriter := multipart.NewWriter(&publishBuffer)

    cfw, err := publishWriter.CreateFormField("public")

    if err != nil {
        log.Fatal(err)
    }

    if _, err = cfw.Write([]byte("1")); err != nil {
        log.Fatal(err)
    }

    cfw, err = publishWriter.CreateFormField("body_top")

    if err != nil {
        log.Fatal(err)
    }

    var topComment string = GenerateTopComment(seenBefore,
                                               requester,
                                               commented,
                                               extraComment)

    if _, err = cfw.Write([]byte(topComment)); err != nil {
        log.Fatal(err)
    }

    cfw, err = publishWriter.CreateFormField("body_top_text_type")

    if err != nil {
        log.Fatal(err)
    }

    if _, err = cfw.Write([]byte("markdown")); err != nil {
        log.Fatal(err)
    }

    if (!seenBefore && config.Comments.Bottom.NewReview != "") {
        cfw, err = publishWriter.CreateFormField("body_bottom")

        if err != nil {
            log.Fatal(err)
        }

        if _, err = cfw.Write([]byte(config.Comments.Bottom.NewReview)); err != nil {
            log.Fatal(err)
        }

        cfw, err = publishWriter.CreateFormField("body_bottom_text_type")

        if err != nil {
            log.Fatal(err)
        }

        if _, err = cfw.Write([]byte("markdown")); err != nil {
            log.Fatal(err)
        }
    }

    publishWriter.Close()

    var reviewUrl string = "http://reviews.example.com/api/review-requests/" +
                           reviewId +
                           "/reviews/" +
                           responseIdStr +
                           "/"

    // Update the review to publish
    req, err := http.NewRequest("PUT", reviewUrl, &publishBuffer)

    req.Header.Add("Authorization", config.RbToken)
    req.Header.Set("Content-Type",
                   publishWriter.FormDataContentType())

    resp, err := (&http.Client{}).Do(req)

    if (err != nil) {
        log.Fatal(err)
    }
    defer resp.Body.Close()

    //TODO - error handling
}

/**
 * Retrieves a review request by its ID.
 *
 * @param reviewId The review ID
 *
 * @retval error, string Any error that occurred, and the review request.
 */
func GetReviewRequest(reviewId string) (error, reviewdata.ReviewRequest) {
    req, err := http.NewRequest("GET",
                                "http://reviews.example.com/api/" +
                                "review-requests/" + reviewId + "/",
                                nil)

    req.Header.Add("Authorization", config.RbToken)

    resp, err := (&http.Client{}).Do(req)

    if (err != nil) {
        log.Fatal(err)
    }
    defer resp.Body.Close()

    body, err := ioutil.ReadAll(resp.Body)

    if (err != nil) {
        log.Fatal(err)
    }

    var review ReviewContainer

    err = json.Unmarshal(body, &review)

    if (err != nil) {
        log.Fatal(err)
    }

    return err, review.Review_Request
}

/**
 * Performs a review.
 *
 * @param incomingReq   The incoming review request.
 * @param reviewPlugins A list of plugins that should be run against the review.
 */
func DoReview(incomingReq   reviewdata.ReviewRequest,
              reviewPlugins []ReviewerPlugin) {
    reviewId := incomingReq.ReviewId
    fmt.Println("Received review request for: " + reviewId)

    var populatedRequest reviewdata.ReviewRequest = incomingReq
    var commentsMade     int = 0

    // If we've not already filled in the request, do that
    if (incomingReq.Id == 0) {
        _, populatedRequest = GetReviewRequest(reviewId)
        populatedRequest.ResultChan = incomingReq.ResultChan
        populatedRequest.SeenBefore = incomingReq.SeenBefore
    }

    // Check if we've seen this diff before
    lastSeenDiff, found := db.KvGet("RLD" + reviewId)

    if (found && lastSeenDiff == populatedRequest.Links.Latest_Diff.Href) {
        // We've already reviewed this before, ignore
        fmt.Println("Ignoring already-seen diff for review " + reviewId)
    } else {
        // Pick up the review's diffs
        err, diff := GetDiffedFiles(populatedRequest.Links.Latest_Diff.Href)

        excludeFileRegex := regexp.MustCompile("TODO - implement")

        //TODO - configuration to pick stuff out of the review title, and pass it to
        //       the checkers
        var diffFiles    []reviewdata.FileDiff

        if (err != nil) {
            // Can't retrieve any files, skip this review
            fmt.Printf("Could not find any files: %s\n", err)
        } else {
            for _, element := range diff.Files {
                _, fileDiff := GetFileDiff(element.Links)

                if (!excludeFileRegex.MatchString(fileDiff.Filename)) {
                    fileDiff.Id = element.Id
                    diffFiles   = append(diffFiles, fileDiff)
                }
            }

            // Create the review reply before processing anything, so we can populate it
            // with comments in parallel
            var responseIdStr = CreateReviewReply(reviewId)

            // Comment on the files
            commentsMade, extraComment := RunCheckersAndComment(reviewId,
                                                                responseIdStr,
                                                                populatedRequest,
                                                                &diffFiles,
                                                                reviewPlugins)

            PublishReview(reviewId,
                          responseIdStr,
                          populatedRequest.Requester,
                          (commentsMade > 0),
                          extraComment,
                          populatedRequest.SeenBefore)

            // Store the fact that we've now reviewed this
            db.KvPut("RLD" + reviewId, populatedRequest.Links.Latest_Diff.Href)

            // Also store some fun stats
            db.KvIncr("reviewsDone", 1)
            db.KvIncr("commentsMade", commentsMade)
        }
    }

    var result reviewdata.ReviewResult
    result.NumComments = int(commentsMade)
    populatedRequest.ResultChan <- result
}

/**
 * Manages reviewer plugins.
 *
 * Reviewer plugins receive file diffs and generate comments on them.
 *
 * @param pluginDir The directory from which requester plugins should be
 *                  loaded.
 */
func LoadReviewerPlugins(pluginDir string) ([]ReviewerPlugin, error) {
    var plugins []ReviewerPlugin

    // Gather all of the files in the plugin directory
    pluginFiles, err := ioutil.ReadDir(pluginDir)

    if (err != nil) {
        fmt.Printf("Could not find any reviewer plugins in %s\n", pluginDir)
        fmt.Println(err)
    } else if (len(pluginFiles) == 0) {
        fmt.Println("Failed to find any reviewer plugins")
        err = errors.New("Failed to find any reviewer plugins")
    } else {
        for _, file := range pluginFiles {
            // Load the plugin
            plug, err := plugin.Open(pluginDir + "/" + file.Name())
            if err != nil {
                fmt.Println(err)
                break
            }

            // Look up the Reviewer symbol, which the plugin must have exported
            reviewPlugin, err := plug.Lookup("ReviewerPlugin")
            if err != nil {
                fmt.Println(err)
                break
            }

            // Assert that the loaded symbol is a ReviewerPlugin
            var reviewer ReviewerPlugin
            reviewer, ok := reviewPlugin.(ReviewerPlugin)
            if !ok {
                fmt.Printf("Could not load Reviewer symbol from %s\n", file)
                break
            }

            // Add the plugin to out list
            plugins = append(plugins, reviewer)

            fmt.Printf("Loaded plugin: %s\n", reviewer.CanonicalName())
        }
    }

    return plugins, err
}

/**
 * Configures the reviewer, given its config block.
 *
 * @param config A raw json message containing config.
 *
 * @retval error Error status
 */
func Configure(rawConfig json.RawMessage) error {
    err := json.Unmarshal(rawConfig, &config)

    return err
}

/**
 * Runs the reviewer. Blocks on the reviewReqs channel, handling reviews as they
 * come in.
 *
 * @param reviewReqs A channel through which review requests are received.
 */
func Go(rawConfig  json.RawMessage,
        reviewReqs <-chan reviewdata.ReviewRequest) {

    err := Configure(rawConfig)

    if (err != nil) {
        log.Fatal(err)
    }

    plugins, err := LoadReviewerPlugins("./plugins/review")

    if (err != nil) {
        log.Fatal(err)
    }

    for {
        go DoReview(<-reviewReqs, plugins)
    }
}
