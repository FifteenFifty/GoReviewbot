package main

import (
	"fmt"
    "flag"
    "io/ioutil"
    "plugin"
    "log"
    "encoding/json"
    "os"
    "runtime"
    "time"

    "rbplugindata/reviewdata"
    "rbbot/reviewer"
    "rbbot/db"
)

/**
 * A ReviewRequester plugin is something that provides the following functions.
 */
type ReviewRequester interface {
    Version()       (int,int,int) // The plguin's version (major minor micro)
    CanonicalName() string // The plugin's canonical name
    Run(chan <- reviewdata.ReviewRequest) // Runs the requester
}

/**
 * The config structure.
 */
type Config struct {
    Version     string          // Config version
    PluginPath  string          // Path under which plugins exist
    DbPath      string          // String to the sqlite database
    ReviewBoard json.RawMessage // Not parsed, passed to the reviewer to parse
    Plugins struct {
        Requester json.RawMessage
        Reviewer  json.RawMessage
    }
    Stats       struct {
        Logstats       bool
        LogIntervalSec int
    }
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
            plug, err := plugin.Open(pluginDir + "/" + file.Name())
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

            fmt.Printf("Loaded requester: %s\n", reviewRequester.CanonicalName())
        }
    }

    return success
}

/**
 * Logs stats every X seconds.
 *
 * @param interval The interval, in seconds, at which stats should be logged.
 */
func LogStats (interval int) {
    for {
        //Print the number of running goroutines
        fmt.Printf("%d goroutines currently running\n", runtime.NumGoroutine())
        time.Sleep(time.Duration(interval) * time.Second)
    }
}

/**
 * Loads configuration.
 */
func LoadConfig (configFile string) Config {
    var config Config

    cfgFile, err := os.Open(configFile)
    defer cfgFile.Close()

    if (err != nil) {
        log.Fatal(err)
    }
    jsonParser := json.NewDecoder(cfgFile)
    jsonParser.Decode(&config)
    return config
}

func main() {
    // Enable better logging
    log.SetFlags(log.LstdFlags | log.Lshortfile)

    fmt.Println("This is main")

    cfgFilePtr := flag.String("cfgFile",
                              "./config.json",
                              "Location of the config file to read")

    flag.Parse()

    fmt.Printf("Config file location: %s\n", *cfgFilePtr)

    config := LoadConfig(*cfgFilePtr)

    // Let the db component know where its database lives
    db.Configure(config.DbPath)

    // A channel into which review requests are placed for reviewing
    reviewRequests := make(chan reviewdata.ReviewRequest)

    if (!RunRequestPlugins(config.PluginPath + "/request", reviewRequests)) {
        log.Fatal("Failed to load request plugins")
    }

    // If stats are enabled, set the stat logger going
    if (config.Stats.Logstats) {
        go LogStats(config.Stats.LogIntervalSec)
    }

    // Set the reviewer going
    reviewer.Go(config.PluginPath + "/review",
                config.ReviewBoard,
                config.Plugins.Reviewer,
                reviewRequests)
}
