package reviewer

func GenerateTopComment(seenBefore   bool,
                        requester    string,
                        commented    bool,
                        extraComment string) string {
    var comment string

    if (!commented) {
        comment = config.Comments.Top.PerfectReview + "\n\n"
    } else {
        if (seenBefore) {
            comment = config.Comments.Top.SeenBefore
        } else {
            comment = config.Comments.Top.NewReview
        } 
        comment += " " + requester + "\n\n"
    }

    if (extraComment != "") {
        comment += "Extra comments:\n\n" + extraComment
    }

    return comment
}
