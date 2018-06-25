// Plugins must be built in the main package
package main

import (
    "rbplugindata/reviewdata"
    "sync"
    "encoding/json"
    "regexp"
    "strings"
    "fmt"
)

type ReviewRegex struct {
    File struct {
        Match   []string
        Exclude []string
    }
    Line struct {
        Match   []string
        Exclude []string
    }
    Comment struct {
        SingleLine string
        MultiLine  string
        RaiseIssue bool
    }
}

type Config struct {
    RegexReviewer struct {
        Checks []ReviewRegex
    }
}

var (
    config Config
)


/**
 * Base plugin struct, to which we'll add methods.
 */
type Reviewer struct {
}

/**
 * Returns the plugin version.
 */
func (p Reviewer) Version() (int, int, int) {
    return 0,0,0
}

/**
 * Returns the plugin's canonical name.
 */
func (p Reviewer) CanonicalName() string {
    return "RegexReviewer"
}

/**
 * Runs each regex on a chunk.
 */
func CheckChunk(regex       ReviewRegex,
                chunk       reviewdata.DiffChunk,
                commentChan chan <- reviewdata.Comment) {
    matchRegex   := regexp.MustCompile(strings.Join(regex.Line.Match, "|"))
    excludeRegex := regexp.MustCompile(strings.Join(regex.Line.Exclude, "|"))
    var comment reviewdata.Comment

    for _, line := range chunk.Lines {
        if (matchRegex.MatchString(line.RhText) &&
            (len(regex.Line.Exclude) == 0 ||
             !excludeRegex.MatchString(line.RhText))) {
            if (comment.NumLines == 0) {
                // First match we've found
                comment.NumLines   = 1
                comment.Line       = line.ReviewLine
                comment.Text       = regex.Comment.SingleLine
                comment.RaiseIssue = regex.Comment.RaiseIssue
                fmt.Println(1)
            } else {
                // Previously matched something
                comment.NumLines += 1
                comment.Text      = regex.Comment.MultiLine
                fmt.Println(2)
            }
        } else {
            // Didn't natch. If we previously did, send the comment
            if (comment.NumLines > 0) {
                commentChan <- comment
                comment.NumLines = 0
                fmt.Println(3)
            }
        }
    }

    // Checked everything. If we've previously set up a comment, send it
    if (comment.NumLines > 0) {
        commentChan <- comment
            fmt.Printf("commented: %s\n", comment.Text)
    }
                fmt.Println(5)
}

/**
 * Runs the plugin on a file.
 */
func (p Reviewer) Check(file        reviewdata.FileDiff,
                        passback    interface{},
                        commentChan chan <- reviewdata.Comment,
                        wg          *sync.WaitGroup) {

    for _, regex := range config.RegexReviewer.Checks {
        matchRegex   := regexp.MustCompile(strings.Join(regex.File.Match, "|"))
        excludeRegex := regexp.MustCompile(strings.Join(regex.File.Exclude,
                                                        "|"))

        if (matchRegex.MatchString(file.Filename) &&
            (len(regex.File.Exclude) == 0 ||
             !excludeRegex.MatchString(file.Filename))) {
            for _, chunk := range file.Diff_Data.Chunks {
                if (chunk.Change == "insert" || chunk.Change == "replace") {
                    CheckChunk(regex, chunk, commentChan)
                }
            }
        }
    }

    (*wg).Done()
}

/**
 * Runs the plugin on a review request.
 */
func (p Reviewer) CheckReview(review      reviewdata.ReviewRequest,
                              commentChan chan <- string) interface{} {
    return nil
}

/**
 * Configures the plugin.
 */
func (p Reviewer) Configure(rawConfig json.RawMessage) {
    json.Unmarshal(rawConfig, &config)
}

// Export our plugin as a ReviewerPlugin for main to pick up
var ReviewerPlugin Reviewer
