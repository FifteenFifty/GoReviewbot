/**
 * Contains configuration structures.
 */
package reviewer

type RbConfig struct {
    RbApiUrl string
    RbToken  string
    RbUsername string /* Used to drop previous comments, when configured to do
                    * so. */
    Comments struct {
        Top struct {
            NewReview     []string
            SeenBefore    []string
            PerfectReview []string
        }
        Bottom struct {
            NewReview  string
            SeenReview string
        }
        DropPreviousComments bool
        MaxComments          int
        MaxCommentComment    string
    }
    ExclusionRegexes struct {
        File        []string
        ReviewTitle []string
    }
    ConcurrentFileDownloads int
    EmailOnPerfect          bool
}
