package main

import (
	"fmt"
    "flag"
    "io/ioutil"
    "plugin"
    "log"

    "rbbot/plugin/reviewdata"
)

/**
 * A ReviewRequester plugin is something that provides the following functions.
 */
type ReviewRequester interface {
    Version()       string // The plguin's version
    CanonicalName() string // The plugin's canonical name
    Run(chan <- reviewdata.ReviewRequest) // Runs the requester
}

/**
 * Manages review request plugins.
 *
 * Review request plugins are responsible for receiving review requests and
 * pushing them into the request channel.
 *
 * @param pluginDir         The directory from which requester plugins should be
 *                          loaded.
 * @param reviewRequestChan The channel into which review requests should be
 *                          pushed.
 *
 * @retval true  If all plugins loaded successfully.
 * @retval false Otherwise (error message will have been printed).
 */
func RunRequestPlugins(pluginDir         string,
                       reviewRequestChan chan <- reviewdata.ReviewRequest) bool {
    var success = true

    // Gather all of the files in the plugin directory
    pluginFiles, err := ioutil.ReadDir(pluginDir)

    if (err != nil) {
        fmt.Printf("Could not find any requester plugins in %s\n", pluginDir)
        fmt.Println(err)
        success = false
    } else if (len(pluginFiles) == 0) {
        fmt.Println("Failed to find any requester plugins")
        success = false
    } else {
        for _, file := range pluginFiles {
            // Load the plugin
            plug, err := plugin.Open(file.Name())
            if err != nil {
                fmt.Println(err)
                success = false
                break
            }

            // Look up the ReviewRequester symbol, which the plugin must have
            // exported
            requester, err := plug.Lookup("ReviewRequester")
            if err != nil {
                fmt.Println(err)
                success = false
                break
            }

            // Assert that the loaded symbol is a ReviewRequester
            var reviewRequester ReviewRequester
            reviewRequester, ok := requester.(ReviewRequester)
            if !ok {
                fmt.Printf("Could not load ReviewRequester symbol from %s\n",
                           file)
                success = false
                break
            }

            // Run the plugin
            go reviewRequester.Run(reviewRequestChan)
        }
    }

    return success
}


func main() {
    fmt.Println("This is main")

    cfgFilePtr := flag.String("cfgFile",
                              "",
                              "Location of the config file to read")

    flag.Parse()

    fmt.Printf("Config file location: %s\n", *cfgFilePtr)

    // A channel into which review requests are placed for reviewing
    reviewRequests := make(chan reviewdata.ReviewRequest)

    if (!RunRequestPlugins("./plugins/request", reviewRequests)) {
        log.Fatal("Failed to load request plugins")
    }
}
