// Plugins must be built in the main package
package main

import (
    "encoding/json"
    "fmt"
    "net/http"
    "strconv"

    "rbplugindata/reviewdata"
)

/**
 * Base plugin struct, to which we'll add methods.
 */
type Requester struct {
}

/**
 * The expected HTTP payload
 */
type Payload struct {
    Secret   string
    ReviewId int
    Force    bool
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

func (p Requester) Configure(json.RawMessage) {
}

/**
 * Runs the plugin.
 */
func (p Requester) Run(reviewRequests chan <- reviewdata.ReviewRequest) {
    http.HandleFunc("/",
                    func(w   http.ResponseWriter,
                         req *http.Request) {
                        defer req.Body.Close()

                        decoder := json.NewDecoder(req.Body)

                        var payload Payload

                        err := decoder.Decode(&payload)

                        if (err != nil) {
                            fmt.Printf("Failed to decode response: %s\n", err)
                        } else if (payload.Secret == "ytfiFneetfiF" &&
                                   payload.ReviewId != 0) {

                            var reviewReq reviewdata.ReviewRequest

                            reviewReq.Force = payload.Force
                            reviewReq.ReviewId = strconv.Itoa(payload.ReviewId)
                            reviewReq.ResultChan = make(
                                                chan reviewdata.ReviewResult,
                                                1)

                            reviewRequests <- reviewReq
                        } else {
                            fmt.Printf("Invalid payload received: %s\n"+
                                       "Decoded into %+v\n", req.Body, payload)
                        }
                    })
    http.ListenAndServe(":1550", nil)
}

// Export our plugin as a ReviewRequester for main to pick up
var ReviewRequester Requester
