// Plugins must be built in the main package
package main

import (
    "rbplugindata/reviewdata"
    "sync"
    "encoding/json"
)

/**
 * Given a chunk of lines, comments on any which are over 80 characters.
 */
func CheckLength(diffChunk   reviewdata.DiffChunk,
                 commentChan chan <- reviewdata.Comment) {
    var comment reviewdata.Comment
    comment.NumLines = 0

    for _, line := range diffChunk.Lines {
        if (len(line.RhText) > 80) {
            comment.NumLines++

            if (comment.NumLines == 1) {
                comment.Line = line.ReviewLine
                comment.Text = "This line is over 80 characters"
            } else {
                comment.Text = "These lines are over 80 characters"
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
    return "LineReviewer"
}

/**
 * Runs the plugin on a file.
 */
func (p Reviewer) Check(file        reviewdata.FileDiff,
                        passback    interface{},
                        commentChan chan <- reviewdata.Comment,
                        wg          *sync.WaitGroup) {

    for _, chunk := range file.Diff_Data.Chunks {
        if (chunk.Change == "insert" || chunk.Change == "replace") {
            CheckLength(chunk, commentChan)
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
func (p Reviewer) Configure(json.RawMessage) {
}

// Export our plugin as a ReviewerPlugin for main to pick up
var ReviewerPlugin Reviewer
