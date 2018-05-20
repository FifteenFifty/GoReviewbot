/**
 * Package contains all of the common data structures that pass through the
 * system. Must be included by all plugins
 * TODO: move into its own package for that?
 */
package reviewdata

import (
    "encoding/json"
    "html"
)

// Contains structs which are used by plugins and passed through main

/**
 * The result of processing a reivew.
 */
type ReviewResult struct {
    NumComments int
}

/**
 * A comment, as returned from review processing.
 */
type Comment struct {
    Line       int    /**< The line on which the comment should be made. Note:
                       *   This is not the line in the modified file, this is
                       *   ReviewBoard's internally-tracked number */
    NumLines   int    /**< The length of the comment. */
    Text       string /**< The comment text. */
    RaiseIssue bool   /**< Whether an issue should be raised alongside the
                       *   comment. */
}

/**
 * A ReviewBoard link.
 */
type Link struct {
    Href   string
    Method string
}

/**
 * ReviewBoard's link contianer
 */
type LinkContainer struct {
    Diffs        Link
    Latest_Diff  Link
    Patched_File Link
    Self         Link
}

/**
 * Container for an entire review request.
 */
type ReviewRequest struct {
    // Fields provided by ReviewBoard
    Id           int
    Summary      string
    Commit_Id    string
    Bugs_Closed  []string
    Links        LinkContainer
    Testing_Done string
    Last_Updated string

    //Fields that are used internally
    ReviewId  string // Populated if the review needs retrieval
    Requester string // The name of the entity that requested the review
               	     // request
    SeenBefore bool  // Whether this review request has been seen before

    /** A  channel into which a ReviewResult shall be pushed when the review
     *  is complete. NOTE: This _must_ be created as a buffered channel. */
    ResultChan chan ReviewResult
}

/**
 * A modified/deleted line.
 */
type Line struct {
    ReviewLine     int    /**< The INTERNAL line against which comments should
                           *   be made. */
    RhLine         int    /**< The line number from the right-hand file in a
                           *   diff. */
    RhText         string /**< The modified line text */
    WhitespaceOnly bool   /**< Whether this line consists of only whitespace
                           *   changes. */
}

/**
 * A review chunk - a set of lines on which the same action is performed.
 */
type DiffChunk struct {
    Index int
    Lines []Line
}

/**
 * The diff of an entire file - consisting of an ordered list of chunks.
 */
type FileDiff struct {
    Id       int
    Filename string

    Diff_Data struct {
        Chunks []DiffChunk
    }

    EntireFile []byte // The whole, raw, file
}

type CommentedFile struct {
    FileId   int
    Comments map[int][]*Comment /**< A map of ints to lists of pointers to
                                 *   comments, used to track comments across
                                 *   lines. */
}

/**
 * Decodes a json object into a Line struct.
 */
func (c *Line) UnmarshalJSON(bs []byte) error {
    arr := []interface{}{}
    json.Unmarshal(bs, &arr)

    // Pick up the internal review line
    reviewLine, ok := arr[0].(float64)
    if (ok) {
        c.ReviewLine     = int(reviewLine)
        c.RhText         = html.UnescapeString(arr[5].(string))
        c.WhitespaceOnly = arr[7].(bool)
    }

    // Pick up the line from the modified file
    rhLine, ok := arr[4].(float64)
    if (ok) {
        c.RhLine = int(rhLine)
    }

    return nil
}
