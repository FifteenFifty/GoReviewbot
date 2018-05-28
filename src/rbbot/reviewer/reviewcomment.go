package reviewer

import (
    "math/rand"
)

func GenerateTopComment(seenBefore   bool,
                        requester    string,
                        commented    bool,
                        extraComment string) string {
    var comment string

    if (seenBefore) {
        comment = config.Comments.Top.SeenBefore[rand.Intn(
                                len(config.Comments.Top.SeenBefore))] + "\n\n"
    } else {
        comment = config.Comments.Top.NewReview[rand.Intn(
                                len(config.Comments.Top.NewReview))] + "\n\n"
    }

    if (!commented) {
        comment += config.Comments.Top.PerfectReview[rand.Intn(
                       len(config.Comments.Top.PerfectReview))] + "\n\n"

    }

    if (extraComment != "") {
        comment += "Extra comments:\n\n" + extraComment
    }

    return comment
}
