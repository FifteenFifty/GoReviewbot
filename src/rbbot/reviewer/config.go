/**
 * Contains configuration structures.
 */
package reviewer

type RbConfig struct {
    RbApiUrl string
    RbToken  string
    Comments struct {
        Top struct {
            NewReview     string
            SeenBefore    string
            PerfectReview string
        }
        Bottom struct {
            NewReview  string
            SeenReview string
        }
    }
    ExclusionRegexes struct {
        File        []string
        ReviewTitle []string
    }
}
