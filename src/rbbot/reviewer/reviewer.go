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
        "strings"
        "time"

        "rbbot/db"
        "rbplugindata/reviewdata"
)

var (
    config                     RbConfig
    fileExclusionRegex        *regexp.Regexp
    fileExclusionsSet          bool
    reviewTitleExclusionRegex *regexp.Regexp
    reviewTitleExclusionSet    bool
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
    Configure(json.RawMessage)    // Configures itself
    Check(reviewdata.FileDiff,
          interface{},
          chan <- reviewdata.Comment,
          *sync.WaitGroup)           // Runs the review plugin on a file
    CheckReview(reviewdata.ReviewRequest,
                chan <- string) interface{} // Runs the review plugin on the
                                            // review request. Returns an
                                            // interface
}

/**
 * When it reviews the review, a plugin can pass back an anonymous structure
 * which we pass back in for file reviews.
 */
type ReviewPluginPassback struct {
    Plugin   ReviewerPlugin
    Passback interface{}
}

/**
 * A key/value pair of strings.
 */
type KvString struct {
    k string
    v string
}

/**
 * Retrieves an object from the ReviewBoard API, and umarshalls it into the
 * passed struct.
 *
 * @param link       The link from which the entity shall be retrieved.
 * @param entity     A pointer to a struct into which the received json shall be
 *                   unmarshsalled.
 * @param addKvStrings Any headers that should be added to the request, on top of
 *                   the ReviewBoard API token.
 *
 * @retval nil   If no error occurred. The entity struct will have been
 *               populated.
 * @retval error If an error occurred. The entity struct will not have been
 *               populated.
 */
func GetEntity(link string, entity interface{}, addKvStrings []KvString) error {
    req, err := http.NewRequest("GET", link, nil)

    req.Header.Add("Authorization", config.RbToken)

    for _, header := range(addKvStrings) {
        req.Header.Add(header.k, header.v)
    }

    resp, err := (&http.Client{}).Do(req)

    if (err != nil) {
        return err
    }
    defer resp.Body.Close()

    body, err := ioutil.ReadAll(resp.Body)

    if (err != nil) {
        return err
    }

    err = json.Unmarshal(body, entity)

    if err != nil {
        return err
    }

    return err
}

/**
 * Retrieves a raw entity from a review, as an array of bytes.
 */
func GetRawEntity(link string) (error, []byte) {
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
 * Sends a request to ReviewBoard.
 *
 * @param method     The rquest method.
 * @param link       The resource to which data shall be sent.
 * @param args       A list of key/value pairs to be added to the request.
 * @param respEntity A pointer to a struct into which the response should be
 *                   decoded. If this is nil, the response is not decoded.
 *
 * @retval nil   If the request was successful.
 * @retval error The error that occurred, if the request was unsuccessful.
 */
func SendRequest(method     string,
                 link       string,
                 args       []KvString,
                 respEntity interface{}) error {
    var b bytes.Buffer

    w := multipart.NewWriter(&b)

    for _, pair := range(args) {
        fw, err := w.CreateFormField(pair.k)

        if err != nil {
            log.Fatal(err)
        }

        _, err = fw.Write([]byte(pair.v))

        if (err != nil) {
            log.Fatal(err)
        }
    }

    w.Close()

    req, err := http.NewRequest(method, link, &b)

    req.Header.Add("Authorization", config.RbToken)
    req.Header.Set("Content-Type", w.FormDataContentType())

    resp, err := (&http.Client{}).Do(req)

    if (err != nil) {
        log.Fatal(err)
    }
    defer resp.Body.Close()

    if (respEntity != nil) {
        body, _ := ioutil.ReadAll(resp.Body)

        err = json.Unmarshal(body, respEntity)

        if err != nil {
            log.Fatal(err)
        }
    }

    return err
}

/**
 * Drops all open comments from a single review reply.
 */
func DropCommentsFromReply(reviewId string, replyId string) {
    type DiffCommentContainer struct {
        Diff_Comments []struct {
            Id           int
            Issue_Opened bool
            Issue_Status string
            Links        struct {
                Self struct {
                    Href string
                }
            }
        }
    }

    var link string = config.RbApiUrl +
                      "/review-requests/" +
                      reviewId +
                      "/reviews/" +
                      replyId +
                      "/diff-comments/"
    var diffCommentContainer DiffCommentContainer

    err := GetEntity(link, &diffCommentContainer, []KvString{})

    if (err != nil) {
        log.Fatal("Could not retrieve diff comments: " + err.Error())
    }

    var toDropList []string

    for _, comment := range(diffCommentContainer.Diff_Comments) {
        if (comment.Issue_Opened && comment.Issue_Status == "open") {
            // This issue is still open. Close it
            toDropList = append(toDropList, comment.Links.Self.Href)
        }
    }

    var wg sync.WaitGroup
    wg.Add(len(toDropList))
    fmt.Printf("There are %d comments to drop from review %s, reply %s\n",
               len(toDropList),
               reviewId,
               replyId)

    // Drop a max of 10 comments at once
    throttleChan := make(chan bool, 10)

    // Now we have a list of all comments to drop, drop them in parallel
    for _, toDrop := range(toDropList) {
        go func (toDropLink string) {
            // Before sending the request, add to the channel. This will
            // block if the channel is full
            throttleChan <- true

            err = SendRequest("PUT",
                              toDropLink,
                              []KvString{{k: "issue_status", v: "dropped"}},
                              nil)
            if (err != nil) {
                log.Fatal("Error while dropping comments: " + err.Error())
            }

            // Eat a value from the throttle channel to free up a space
            _ = <- throttleChan

            wg.Done()
        }(toDrop)
    }
    wg.Wait()
    fmt.Printf("All comments are dropped from review %s\n", reviewId)
}

/**
 * Drops previous review comments made by the bot. Returns when all comments are
 * dropped.
 */
func DropPreviousComments(reviewId string) {
    // If we've logged the last reply we made, drop the comments from that
    lastReplyId, found := db.KvGet("LastReplyId_" + reviewId)

    if (found) {
        DropCommentsFromReply(reviewId, lastReplyId)
    } else {
        // This review was last reviewed by a previous version of the bot.
        // Search the entire list of replies for any that it made
        type ReplyContainer struct {
            Reviews []struct {
                Id    int
                Links struct {
                    User struct {
                        Title string
                    }
                }
            }
        }

        var replyContainer ReplyContainer
        var url string = config.RbApiUrl +
                            "/review-requests/" +
                            reviewId +
                            "/reviews/"

        err := GetEntity(url, &replyContainer, []KvString{})

        if (err != nil) {
            log.Fatal("Could not retrieve review response list: " + err.Error())
        }

        for _, reply := range(replyContainer.Reviews) {
            if (reply.Links.User.Title == config.RbUsername) {
                // This is one of ours
                DropCommentsFromReply(reviewId, strconv.Itoa(reply.Id))
            }
        }
    }
}


/**
 * Retrieves diffed files for a review
 */
func GetDiffedFiles(link string) (error, DiffFileContainer) {
    var diffFiles DiffFileContainer
    err := GetEntity(link + "/files/", &diffFiles, []KvString{})
    return err, diffFiles
}

/**
 * Retrieves a single file diff from a review's file links.
 */
func GetFileDiff (links reviewdata.LinkContainer) (error, reviewdata.FileDiff) {
    var file     reviewdata.FileDiff
    var fileData ReviewFileData

    err := GetEntity(links.Self.Href,
                    &file,
                    []KvString{
                        {k: "Accept",
                         v: "application/vnd.reviewboard.org.diff.data+json"}})

    if err != nil {
        log.Fatal(err)
    }

    err = GetEntity(links.Self.Href, &fileData, []KvString{})

    if ( err != nil) {
        log.Fatal(err)
    }

    err, entireFile := GetRawEntity(links.Patched_File.Href)

    if ( err != nil) {
        log.Fatal(err)
    }

    file.Filename   = fileData.File.Dest_File
    file.EntireFile = entireFile

    return err, file
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

        commentList, exists := (*outComments).Comments[comment.Line]

        if (exists) {
            if (len(commentList) == 0) {
                commentList = append(commentList, &comment)
                added = true
            } else {
                for i := 0; i < len(commentList); i++ {
                    if (commentList[i].NumLines == comment.NumLines) {

                        commentList[i].Text += "\n\n" + comment.Text
                        if (comment.RaiseIssue) {
                            commentList[i].RaiseIssue = true
                        }

                        added = true
                    }
                }

                if (!added) {
                    commentList = append(commentList, &comment)
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
func CheckFileAndComment(file            reviewdata.FileDiff,
                         reviewIdStr     string,
                         responseIdStr   string,
                         commentCount   *int32,
                         wg             *sync.WaitGroup,
                         reviewPlugins  []ReviewPluginPassback) {
    timer := time.Now()

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
        go reviewPlugins[i].Plugin.Check(file,
                                         reviewPlugins[i].Passback,
                                         comments,
                                         &checkerGroup)
    }

    // Wait for them all to complete
    checkerGroup.Wait()

    // Close the comment stream when all of the checkers are done. The comment
    // manager will then exit - wait for it to exit, so we're sure that all of
    // the comments have been processed
    close(comments)

    commentMgrWg.Wait()

    fmt.Printf("Running checkers took: %s\n", time.Since(timer))

    // If there are comments on the file, add them to the review
    if (len(commentedFile.Comments) > 0) {
        // As this is running in parallel with all of the other checks, we can't
        // check existing value and update it in two steps. Everyone adds their
        // count to the total and then subtracts it to get the original value
        totalComments := atomic.AddInt32(commentCount,
                                         int32(len(commentedFile.Comments)))

        // The number of comments we're allowed to make is:
        allowedComments := config.Comments.MaxComments -
                           (int(totalComments) -
                            len(commentedFile.Comments))

        log.Printf("We are allowed %d max comments on %d\n",
                   config.Comments.MaxComments,
                   commentedFile.FileId)
        log.Printf("We have left %d comments on %d\n",
                   (int(totalComments) - len(commentedFile.Comments)),
                   commentedFile.FileId)
        log.Printf("We are allowed %d comments on %d\n",
                   allowedComments,
                   commentedFile.FileId)

        // If we've some comment budget left, send the all of the comments
        if (allowedComments > 0) {
            SendFileComments(reviewIdStr,
                             responseIdStr,
                             commentedFile,
                             allowedComments)
        }
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
    var fileCheckWaitGroup sync.WaitGroup
    var commentsMade       int32 = 0

    fileCheckWaitGroup.Add(len(*files))

    var reviewCommentChan chan string = make(chan string, len(reviewPlugins))

    var pluginPassbacks []ReviewPluginPassback

    for i := 0; i < len(reviewPlugins); i++ {
        // Check the review synchronously, allowing the plugin to pass back
        // something that we give to its file checks
        var passback ReviewPluginPassback
        passback.Plugin = reviewPlugins[i]
        passback.Passback = reviewPlugins[i].CheckReview(reviewRequest,
                                                         reviewCommentChan)
        pluginPassbacks = append(pluginPassbacks, passback)
    }

    for i := 0; i < len(*files); i++ {
        go CheckFileAndComment((*files)[i],
                               reviewIdStr,
                               responseIdStr,
                               &commentsMade,
                               &fileCheckWaitGroup,
                               pluginPassbacks)
    }

    // Wait for all file checks to complete
    fileCheckWaitGroup.Wait()

    var generalComment string = ""

    for i := 0; i < len(reviewCommentChan); i++ {
        generalComment += <-reviewCommentChan + "\n"
    }

    commentsGenerated := int(atomic.LoadInt32(&commentsMade))

    if (commentsGenerated > config.Comments.MaxComments) {
        generalComment += "\n" + config.Comments.MaxCommentComment + "\n"
    }

    return commentsGenerated, generalComment
}

/**
 * Creates an empty review reply, to which comments can be attached.
 *
 * @param reviewId The review ID.
 *
 * @retval string The ID of the review reply, as a string.
 */
func CreateReviewReply (reviewId string) string {
    var reviewUrl string = config.RbApiUrl +
                           "/review-requests/" +
                           reviewId +
                           "/reviews/"

    var reviewResponse ReviewResponse

    err := SendRequest("POST",
                       reviewUrl,
                       []KvString{{k: "body_top", v: "This is a test review"}},
                       &reviewResponse)

    if (err != nil) {
        log.Fatal(err)
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
 * @param allowedComments        The maximum number of comments we are allowed
 *                               to send.
 */
func SendFileComments(reviewId               string,
                      reviewResponseIdString string,
                      comments               reviewdata.CommentedFile,
                      allowedComments        int) {

    var commentsMade int = 0

    if (len(comments.Comments) > 0) {
        // Multiple comments can start on the same line and span different
        // numbers of lines
        for line, commentList := range comments.Comments {
            for _, comment := range commentList {
                // If we've sent as many as we're allowed, don't send any more
                if (commentsMade >= allowedComments) {
                    break
                }

                var commentsId  string = strconv.Itoa(comments.FileId)
                var commentLine string = strconv.Itoa(line)
                var numLines    string = strconv.Itoa(comment.NumLines)
                var raiseIssue  string

                if (comment.RaiseIssue) {
                    raiseIssue = "true"
                } else {
                    raiseIssue = "false"
                }

                var reviewCommentUrl string = config.RbApiUrl +
                                              "/review-requests/" +
                                              reviewId +
                                              "/reviews/" +
                                              reviewResponseIdString +
                                              "/diff-comments/"

                SendRequest("POST",
                            reviewCommentUrl,
                            []KvString{{k: "filediff_id",  v: commentsId},
                                       {k: "first_line",   v: commentLine},
                                       {k: "num_lines",    v: numLines},
                                       {k: "text",         v: comment.Text},
                                       {k: "issue_opened", v: raiseIssue}},
                            nil)
                commentsMade++
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
 *
 * @retval nil   On success.
 * @retval error If an error occurred while publishing.
 */
func PublishReview(reviewId      string,
                   responseIdStr string,
                   requester     string,
                   commented     bool,
                   extraComment  string,
                   seenBefore    bool) error {

    var topComment string = GenerateTopComment(seenBefore,
                                               requester,
                                               commented,
                                               extraComment)

    var kvReq []KvString

    kvReq = append(kvReq,
                   KvString{k: "public",             v: "1"},
                   KvString{k: "body_top",           v: topComment},
                   KvString{k: "body_top_text_type", v: "markdown"})

    if (!config.EmailOnPerfect && !commented) {
        kvReq = append(kvReq, KvString{k: "trivial", v: "true"})
    }

    if (!seenBefore && config.Comments.Bottom.NewReview != "") {
        var bottomComment string = config.Comments.Bottom.NewReview
        kvReq = append(kvReq,
                       KvString{k: "body_bottom",           v: bottomComment},
                       KvString{k: "body_bottom_text_type", v: "markdown"})
    }

    var reviewUrl string = "http://reviews.example.com/api/review-requests/" +
                           reviewId +
                           "/reviews/" +
                           responseIdStr +
                           "/"

    err := SendRequest("PUT",
                       reviewUrl,
                       kvReq,
                       nil)
    return err
}

/**
 * Retrieves a review request by its ID.
 *
 * @param reviewId The review ID
 *
 * @retval Any error that occurred, and the review request.
 */
func GetReviewRequest(reviewId string) (reviewdata.ReviewRequest, error) {
    var url string = config.RbApiUrl + "/review-requests/" + reviewId + "/"

    var review ReviewContainer

    err := GetEntity(url, &review, []KvString{})

    return review.Review_Request, err
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

    timer := time.Now()
    totalTime := time.Now()

    var populatedRequest reviewdata.ReviewRequest = incomingReq
    var commentsMade     int = 0
    var err              error

    // If we've not already filled in the request, do that
    if (incomingReq.Id == 0) {
        populatedRequest, err = GetReviewRequest(reviewId)

        populatedRequest.ResultChan = incomingReq.ResultChan
        populatedRequest.Force      = incomingReq.Force

        if (err != nil) {
            // Something went wrong loading the review
            fmt.Println("Failed to process review")
            fmt.Println(err)
        }
    }

    // Check if we've seen this diff before
    lastSeenDiff, found := db.KvGet("RLD" + reviewId)

    if (populatedRequest.Id != 0) {
        if (found &&
            populatedRequest.Force == false &&
            lastSeenDiff == populatedRequest.Links.Latest_Diff.Href) {
            // We've already reviewed this before, ignore
            fmt.Println("Ignoring already-seen diff for review " + reviewId)
        } else if (reviewTitleExclusionSet &&
                   populatedRequest.Force == false &&
                   reviewTitleExclusionRegex.MatchString(
                                                        populatedRequest.Summary)) {
            // We've excluded this review by title
            fmt.Println("Ignoring review by title: " + populatedRequest.Summary)
        } else {
            // If we found a latest diff URL, we've seen this review before
            populatedRequest.SeenBefore = found

            // Store the fact that we've now seen this diff
            db.KvPut("RLD" + reviewId, populatedRequest.Links.Latest_Diff.Href)

            // If configured to do so, drop all of our previous comments
            if (config.Comments.DropPreviousComments &&
                populatedRequest.SeenBefore) {

                timer = time.Now()
                DropPreviousComments(reviewId)
                fmt.Printf("Dropping previous comments took %s\n",
                           time.Since(timer))
                timer = time.Now()
            }

            // Pick up the review's diffs
            err, diff := GetDiffedFiles(populatedRequest.Links.Latest_Diff.Href)

            var diffFiles    []reviewdata.FileDiff

            if (err != nil) {
                // Can't retrieve any files, skip this review
                fmt.Printf("Could not find any files: %s\n", err)
            } else {
                // We retrieve the files in parallel (up to x at a time), and
                // need to mutex the list addition because slice appending is
                // not goroutine-safe
                var fileWaiter    sync.WaitGroup
                var fileListMutex sync.Mutex
                throttleChan := make(chan bool, config.ConcurrentFileDownloads)
                fileWaiter.Add(len(diff.Files))

                for _, passToFunc := range diff.Files {
                    go func (diffFile DiffFile) {
                        // Before retrieving the file, add to the channel. This
                        // will block if the channel is full
                        throttleChan <- true

                        _, fileDiff := GetFileDiff(diffFile.Links)

                        if (!fileExclusionRegex.MatchString(fileDiff.Filename)) {
                            fileDiff.Id = diffFile.Id
                            fileListMutex.Lock()
                                diffFiles   = append(diffFiles, fileDiff)
                            fileListMutex.Unlock()
                        }
                        fileWaiter.Done()

                        // Eat a value from the throttle channel to free up a
                        // space
                        _ = <- throttleChan
                    }(passToFunc)
                }

                // Wait for all files to have been retrieved
                fileWaiter.Wait()

                fmt.Printf("Retrieving the review took %s\n", time.Since(timer))
                timer = time.Now()

                // Create the review reply before processing anything, so we can populate it
                // with comments in parallel
                var responseIdStr = CreateReviewReply(reviewId)

                // Save the reply ID in case we review this again
                db.KvPut("LastReplyId_" + reviewId, responseIdStr)

                fmt.Printf("Making the reply took %s\n", time.Since(timer))
                timer = time.Now()

                // Comment on the files
                commentsMade, extraComment := RunCheckersAndComment(reviewId,
                                                                    responseIdStr,
                                                                    populatedRequest,
                                                                    &diffFiles,
                                                                    reviewPlugins)
                fmt.Printf("Commenting took %s\n", time.Since(timer))
                timer = time.Now()

                PublishReview(reviewId,
                              responseIdStr,
                              populatedRequest.Requester,
                              (commentsMade > 0),
                              extraComment,
                              populatedRequest.SeenBefore)

                fmt.Printf("Publishing took %s\n", time.Since(timer))
                timer = time.Now()

                // Also store some fun stats
                db.KvIncr("reviewsDone", 1)
                db.KvIncr("commentsMade", commentsMade)

                fmt.Printf("Databasing took %s\n", time.Since(timer))
                timer = time.Now()
            }
        }
    } else {
        fmt.Println("Could not retrieve the review request")
    }

    var result reviewdata.ReviewResult
    result.NumComments = int(commentsMade)
    populatedRequest.ResultChan <- result

    fmt.Printf("All done after only %s\n", time.Since(totalTime))
}

/**
 * Manages reviewer plugins.
 *
 * Reviewer plugins receive file diffs and generate comments on them.
 *
 * @param pluginDir The directory from which requester plugins should be
 *                  loaded.
 * @param pConfig   A raw json message containing config which plugins will
 *                  decode.
 */
func LoadReviewerPlugins(pluginDir string,
                         pConfig json.RawMessage) ([]ReviewerPlugin, error) {
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

            // Configure the plugin
            reviewer.Configure(pConfig)

            // Add the plugin to out list
            plugins = append(plugins, reviewer)

            major, minor, micro := reviewer.Version()
            fmt.Printf("Loaded plugin: %s at version %d.%d.%d\n",
                       reviewer.CanonicalName(),
                       major,
                       minor,
                       micro)
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

    // Build the file exclusion regex
    if (len(config.ExclusionRegexes.File) > 0) {
        fileExclusionRegex = regexp.MustCompile(
                                strings.Join(
                                    config.ExclusionRegexes.File,
                                    "|"))
        fileExclusionsSet = true
    } else {
        fileExclusionsSet = false
    }

    // Build the review title exclusion regex
    if (len(config.ExclusionRegexes.ReviewTitle) > 0) {
        reviewTitleExclusionRegex = regexp.MustCompile(
                            strings.Join(config.ExclusionRegexes.ReviewTitle,
                                         "|"))
        reviewTitleExclusionSet = true
    } else {
        reviewTitleExclusionSet = false
    }

    return err
}

/**
 * Runs the reviewer. Blocks on the reviewReqs channel, handling reviews as they
 * come in.
 *
 * @param pluginPath            The path to the directory in which reviewer
 *                              plugins shall be found.
 * @param rawConfig             A json-encoded struct containing reviewer
 *                              configuration.
 * @param reviewPluginRawConfig A json-encoded struct which is passed to
 *                              plugins, and from which they configure.
 * @param reviewReqs            A channel through which review requests are
 *                              received.
 */
func Go(pluginPath            string,
        rawConfig             json.RawMessage,
        reviewPluginRawConfig json.RawMessage,
        reviewReqs            <-chan reviewdata.ReviewRequest) {

    err := Configure(rawConfig)

    fmt.Printf("Reviewer config: %+v\n", config)

    if (err != nil) {
        log.Fatal(err)
    }

    plugins, err := LoadReviewerPlugins(pluginPath, reviewPluginRawConfig)

    if (err != nil) {
        log.Fatal(err)
    }

    for {
        go DoReview(<-reviewReqs, plugins)
    }
}
