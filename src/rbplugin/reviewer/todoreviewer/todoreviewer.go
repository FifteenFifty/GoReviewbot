// Plugins must be built in the main package
package main

import (
    "rbplugindata/reviewdata"
    "sync"
    "strings"
    "encoding/json"
)

type Config struct {
    TodoReviewer struct {
        Comment string
    }
}

var (
    config Config
)

/**
 * Given a chunk of lines, comments on any which contain TODOs.
 */
func CheckTodos(diffChunk   reviewdata.DiffChunk,
                commentChan chan <- reviewdata.Comment) {
    var comment reviewdata.Comment
    comment.NumLines = 0

    for _, line := range diffChunk.Lines {
        if (strings.Contains(line.RhText, "TODO")) {
            comment.NumLines++

            if (comment.NumLines == 1) {
                comment.Line = line.ReviewLine
                comment.Text = config.TodoReviewer.Comment
                comment.RaiseIssue = true
            } else {
                comment.Text = "These lines contain TODOs"
            }
        } else if (comment.NumLines > 0) {
            commentChan <- comment
            comment.NumLines = 0
        }
    }

    if (comment.NumLines > 0) {
        commentChan <- comment
    }
}

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
    return "TodoReviewer"
}

/**
 * Runs the plugin on a file.
 */
func (p Reviewer) Check(file        reviewdata.FileDiff,
                        commentChan chan <- reviewdata.Comment,
                        wg          *sync.WaitGroup) {

    for _, chunk := range file.Diff_Data.Chunks {
        if (chunk.Change == "insert" || chunk.Change == "replace") {
            CheckTodos(chunk, commentChan)
        }
    }

    (*wg).Done()
}

/**
 * Runs the plugin on a review request.
 */
func (p Reviewer) CheckReview(review      reviewdata.ReviewRequest,
                              commentChan chan <- string,
                              wg          *sync.WaitGroup) {

    (*wg).Done()
}

/**
 * Configures the plugin.
 */
func (p Reviewer) Configure(rawConfig json.RawMessage) {
    json.Unmarshal(rawConfig, &config)
}

// Export our plugin as a ReviewerPlugin for main to pick up
var ReviewerPlugin Reviewer
