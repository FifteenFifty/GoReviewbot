// Plugins must be built in the main package
package main

import (
    "net/http"

    "rbbot/plugin/reviewdata"
)

/**
 * Base plugin struct, to which we'll add methods.
 */
type Requester struct {
}

/**
 * Returns the plugin version.
 */
func (p Requester) Version() (int, int, int) {
    return 0,0,0
}

/**
 * Returns the plugin's canonical name.
 */
func (p Requester) CanonicalName() string {
    return "HttpRequester"
}

/**
 * Runs the plugin.
 */
func (p Requester) Run(reviewRequests chan <- reviewdata.ReviewRequest) {
    http.HandleFunc("/",
                    func(w   http.ResponseWriter,
                         req *http.Request) {
                        var reviewReq reviewdata.ReviewRequest

                        reviewReq.ReviewId = "1"

                        reviewRequests <- reviewReq
                    })
    http.ListenAndServe(":1550", nil)
}

// Export our plugin as a ReviewRequester for main to pick up
var ReviewRequester Requester
