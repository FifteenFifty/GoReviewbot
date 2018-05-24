package reviewer

func GenerateTopComment(seenBefore   bool,
                        requester    string,
                        commented    bool,
                        extraComment string) string {
    var comment string

    if (seenBefore) {
        comment = config.Comments.Top.SeenBefore + "\n\n"
    } else {
        comment = config.Comments.Top.NewReview + "\n\n"
    }

    if (!commented) {
        comment += config.Comments.Top.PerfectReview + "\n\n"
    }

    if (extraComment != "") {
        comment += "Extra comments:\n\n" + extraComment
    }

    return comment
}
