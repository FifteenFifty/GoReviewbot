# Go Review Bot

Automatically performs ReviewBoard code reviews, highlighting common mistakes
and performing "grunt-work", such that human reviewers can focus on the
intelligent stuff.

A plug-in architecture is used for review request generation and code review.

- Requester plugins generate review requests.
- Reviewer plugins review files, as well as the review request itself, and
  generate comments.
- Comments are collated and posted to the review

# Dependencies

go-sqlite3:
    https://github.com/mattn/go-sqlite3

```
export GOPATH=`pwd` && go get github.com/mattn/go-sqlite3
```

# Plugins

Code review is handled through plugins. The idea is that "reviewer" development
is then separate from "core", since core remains constant across ReviewBoard
instances and coding standards differ. It also means that plugins can be
trivially developed, and there's no chance of accidentally breaking core.

## Requester Plugins

Requester plugins are responsible for generating review requests.

A ReviewRequester has the following signature:

```
type ReviewRequester interface {
    Version()       (int,int,int) // The plguin's version (major minor micro)
    CanonicalName() string        // The plugin's canonical name

    Run(chan <- reviewdata.ReviewRequest)
}
```

Version and CanonicalName are run once when the plugin is loaded. The
CanonicalName must be unique across plugins of the same type (it's used for
error reporting).

`Run` is executed as a goroutine, so is expected to continue indefinitely and
block while waiting for review requests.

To generate a review request, the Requester must create a
reviewdata.ReviewRequest struct and populate it with at least:

```
ReviewId   <The ID of a review which requires reviewing>
ResultChan <A buffered channel into which the review result shall be placed>
```

Note: If `Id` is populated, it is assumed that the ReviewRequest has been fully
populated. If `ReviewId` is populated and `Id` is zero, the bot will populate
the ReviewRequest itself.


## Reviewer plugins

Reviewer plugins are responsible for detecting issues and commenting on files.

A Reviewer has the following signature:

```
type ReviewerPlugin interface {
    Version()       (int,int,int) // The plguin's version (major minor micro)
    CanonicalName() string        // The plugin's canonical name

    Check(reviewdata.FileDiff,        //[IN]
          chan <- reviewdata.Comment, //[OUT]
          *sync.WaitGroup)            // Runs the review plugin on a file
    CheckReview(reviewdata.ReviewRequest,
                chan <- string,
                *sync.WaitGroup) // Runs the review plugin on the review request
}
```

Version and CanonicalName are run once when the plugin is loaded. The
CanonicalName must be unique across plugins of the same type (it's used for
error reporting).

`Check` is executed once per file being reviewed. It does the following:
- Receives the file being reviewed
- Generates Comments on the file, and pushes them into the passed channel
    - The Reviewer groups comments such that there is at most one per line of
      the file being reviewed
- [Required] Informs the reviewer that it has finished reviewing the file, by
  calling Done on the passed WaitGroup. This must be done even if there were no
  comments generated

`CheckReview` is executed once. It does the following:
- Receives the ReviewRequest
- Generates comments on the file, in the form of strings, and pushes them into
  the passed channel
    - The Reviewer adds all review comments to the top of its review
- [Required] Informs the Reviewer that it has finished reviewing the review
  request, by calling Done on the passed WaitGroup. This must be done even if
  there were no comments generated
