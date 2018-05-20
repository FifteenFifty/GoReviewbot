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

        "rbbot/plugin/reviewdata"
)

type DiffFile struct {
    Id    int
    Links reviewdata.LinkContainer
}

type DiffFileContainer struct {
    Files []DiffFile
}

type ReviewFileData struct {
    File struct {
        Dest_File string
    }
}

type ReviewResponse struct {
    Review struct {
        Id int
    }
}

type ReviewContainer struct {
    Stat           string
    Review_Request reviewdata.ReviewRequest
}

/**
 * Retrieves diffed files for a review
 */
func GetDiffedFiles(link string) (error, DiffFileContainer) {
    req, err := http.NewRequest("GET", link + "/files/", nil)

    req.Header.Add("Authorization",
                   "token 940121df848fabb83c5b02a66b6ed1da513c78ff")

    resp, err := (&http.Client{}).Do(req)

    if (err != nil) {
        log.Fatal(err)
    }
    defer resp.Body.Close()

    body, err := ioutil.ReadAll(resp.Body)

    if (err != nil) {
        log.Fatal(err)
    }

    fmt.Printf("The body is: %s\n\n", body)

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

func GetFileDiff (links reviewdata.LinkContainer) (error, reviewdata.FileDiff) {
    req, err := http.NewRequest("GET", links.Self.Href, nil)

    req.Header.Add("Authorization",
                   "token 940121df848fabb83c5b02a66b6ed1da513c78ff")
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

    fmt.Printf("The body is: %s\n\n", body)

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

    err, entireFile := GetEntireFile(links.Patched_File.Href)

    if ( err != nil) {
        log.Fatal(err)
    }

    file.Filename   = reviewFileData.File.Dest_File
    file.EntireFile = entireFile

    return err, file

}

func GetEntireFile(link string) (error, []byte) {
    req, err := http.NewRequest("GET", link, nil)

    req.Header.Add("Authorization",
                   "token 940121df848fabb83c5b02a66b6ed1da513c78ff")

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

func GetFileData(link string) (error, ReviewFileData) {
    req, err := http.NewRequest("GET", link, nil)

    req.Header.Add("Authorization",
                   "token 940121df848fabb83c5b02a66b6ed1da513c78ff")

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

func CheckFileAndComment(file           reviewdata.FileDiff,
                         reviewIdStr    string,
                         responseIdStr  string,
                         commentCount  *int32,
                         wg            *sync.WaitGroup) {
    // Count the plugins
    var numCheckers = 1

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

func RunCheckersAndComment(reviewIdStr    string,
                           responseIdStr  string,
                           files         *[]reviewdata.FileDiff) (int32) {
    var fileCheckWaitGroup sync.WaitGroup
    var commentsMade int32 = 0

    fileCheckWaitGroup.Add(len(*files))

    for i := 0; i < len(*files); i++ {
        go CheckFileAndComment((*files)[i],
                               reviewIdStr,
                               responseIdStr,
                               &commentsMade,
                               &fileCheckWaitGroup)
    }

    // Wait for all file checks to complete
    fileCheckWaitGroup.Wait()

    return atomic.LoadInt32(&commentsMade)
}






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

    var reviewUrl string = "http://reviews.example.com/api/review-requests/" +
                           reviewId +
                           "/reviews/"

    fmt.Printf("\n\nURL: %s", reviewUrl)

    // Post a new blank review, to which we will add comments
    req, err := http.NewRequest("POST", reviewUrl, &b)

    req.Header.Add("Authorization",
                   "token 940121df848fabb83c5b02a66b6ed1da513c78ff")
    req.Header.Set("Content-Type",
                   w.FormDataContentType())

    fmt.Printf("Content type: %s", w.FormDataContentType())

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

                fmt.Printf("Sending: %+v\n\n", commentBuffer)

                // Post the comments
                req, err := http.NewRequest("POST",
                                            reviewCommentUrl,
                                            &commentBuffer)

                req.Header.Add("Authorization",
                               "token 940121df848fabb83c5b02a66b6ed1da513c78ff")
                req.Header.Set("Content-Type",
                               commentWriter.FormDataContentType())

                resp, err := (&http.Client{}).Do(req)

                if (err != nil) {
                    log.Fatal(err)
                }
                defer resp.Body.Close()

                body, _ := ioutil.ReadAll(resp.Body)

                fmt.Printf("Response: %s\n\n", body)
            }
        }
    }
}

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

    var topComment string = GenerateTopComment(requester,
                                               commented,
                                               extraComment,
                                               seenBefore)

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

    if (!seenBefore) {
        cfw, err = publishWriter.CreateFormField("body_bottom")

        if err != nil {
            log.Fatal(err)
        }

        if _, err = cfw.Write([]byte(GenerateBottomComment())); err != nil {
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

    req.Header.Add("Authorization",
                   "token 940121df848fabb83c5b02a66b6ed1da513c78ff")
    req.Header.Set("Content-Type",
                   publishWriter.FormDataContentType())

    resp, err := (&http.Client{}).Do(req)

    if (err != nil) {
        log.Fatal(err)
    }
    defer resp.Body.Close()

    body, _ := ioutil.ReadAll(resp.Body)

    fmt.Printf("Response: %s\n\n", body)
}

func GetReviewRequest(reviewId string) (error, reviewdata.ReviewRequest) {
    req, err := http.NewRequest("GET",
                                "http://reviews.example.com/api/" +
                                "review-requests/" + reviewId + "/",
                                nil)

    req.Header.Add("Authorization",
                   "token 940121df848fabb83c5b02a66b6ed1da513c78ff")

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

func DoReview(incomingReq reviewdata.ReviewRequest) {
    reviewId := incomingReq.ReviewId

    var populatedRequest reviewdata.ReviewRequest = incomingReq

    // If we've not already filled in the request, do that
    if (incomingReq.Id == 0) {
        _, populatedRequest = GetReviewRequest(reviewId)
        populatedRequest.ResultChan = incomingReq.ResultChan
        populatedRequest.SeenBefore = incomingReq.SeenBefore
    }

    // Pick up the review's diffs
    err, diff := GetDiffedFiles(populatedRequest.Links.Latest_Diff.Href)

    excludeFileRegex := regexp.MustCompile("TODO - implement")

    //TODO - configuration to pick stuff out of the review title, and pass it to
    //       the checkers
    var diffFiles    []reviewdata.FileDiff
    var commentsMade int = 0

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
        commentsMade = int(RunCheckersAndComment(reviewId,
                                                 responseIdStr,
                                                 &diffFiles))

        //TODO - allow checkers to populate the extra comment
        var extraComment string = "TODO"

        PublishReview(reviewId,
                      responseIdStr,
                      populatedRequest.Requester,
                      (commentsMade > 0),
                      extraComment,
                      populatedRequest.SeenBefore)

        //TODO - store stats
    }

    var result reviewdata.ReviewResult
    result.NumComments = int(commentsMade)
    populatedRequest.ResultChan <- result
}

func Go(reviewReqs chan reviewdata.ReviewRequest) {
    for {
        go DoReview(<-reviewReqs)
    }
}
