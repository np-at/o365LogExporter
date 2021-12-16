package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/cornelk/hashmap"
	"github.com/urfave/cli/v2"
	"github.com/urfave/cli/v2/altsrc"
	"log"
	"net/http"
	"net/url"
	"o365logexporter/promtail-client/promtail"
	"os"
	"strconv"
	"sync"
	"time"
)

const MaxEntriesChanSize = 10000
const lokiAddressFlag = "LokiAddress"

var TenantID string      // See https://docs.microsoft.com/en-us/azure/azure-resource-manager/resource-group-create-service-principal-portal#get-tenant-id
var ApplicationID string // See https://docs.microsoft.com/en-us/azure/azure-resource-manager/resource-group-create-service-principal-portal#get-application-id-and-authentication-key
var ClientSecret string  //
var pubId string
var currentTime time.Time
var currentTimeUnixString string
var chunkDuration time.Duration
var chunkCount int

func main() {
	currentTime = time.Now().UTC()
	currentTimeUnixString = strconv.FormatInt(currentTime.Unix(), 10)
	chunkDuration = time.Hour * 2
	chunkCount = 1
	flags := []cli.Flag{
		&cli.StringFlag{
			Name: "load",
		},
		&cli.BoolFlag{
			Name:    "daemonize",
			Aliases: []string{"z"},
		},
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:    "RunInterval",
			Value:   "5m",
			EnvVars: []string{"APP_RUN_INTERVAL"},
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        "TenantId",
			Destination: &TenantID,
			Aliases:     []string{"t"},
			EnvVars:     []string{"APP_TENANT_ID"},
			//Required:    true,
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        "ApplicationId",
			Aliases:     []string{"a"},
			Destination: &ApplicationID,
			EnvVars:     []string{"APP_APPLICATION_ID"},
			//Required:    true,
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        "ClientSecret",
			Aliases:     []string{"c"},
			Destination: &ClientSecret,
			EnvVars:     []string{"APP_CLIENT_SECRET"},
			//Required:    true,
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:      "output_file",
			Aliases:   []string{"f"},
			TakesFile: true,
			Required:  false,
			EnvVars:   []string{"APP_OUTPUT_FILE"},
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:     "PublisherId",
			Required: false,
			EnvVars:  []string{"APP_PUBLISHER_ID"},
		}),
		altsrc.NewBoolFlag(&cli.BoolFlag{
			Name:    "General",
			EnvVars: []string{"APP_GENERAL"},
		}),

		altsrc.NewBoolFlag(&cli.BoolFlag{
			Name:    "Exchange",
			Aliases: []string{"exchange"},
			EnvVars: []string{"APP_EXCHANGE"},
		}),
		altsrc.NewBoolFlag(&cli.BoolFlag{
			Name:    "Azure Ad",
			Aliases: []string{"azuread"},
			EnvVars: []string{"APP_AZUREAD"},
		}),
		altsrc.NewBoolFlag(&cli.BoolFlag{
			Name:    "Sharepoint",
			EnvVars: []string{"APP_SHAREPOINT"},
		}),
		altsrc.NewBoolFlag(&cli.BoolFlag{Name: "DLP",
			EnvVars: []string{"APP_DLP"}}),

		altsrc.NewBoolFlag(&cli.BoolFlag{
			Name:    "debug",
			Aliases: []string{"d"},
			EnvVars: []string{"APP_DEBUG"},
		}),
		altsrc.NewStringSliceFlag(&cli.StringSliceFlag{
			Name:    "StaticLabel",
			Aliases: []string{"l"},
			EnvVars: []string{"APP_STATIC_LABELS"},
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:      "HistoryFile",
			TakesFile: true,
			EnvVars:   []string{"APP_HISTORY_FILE"},
			Value:     ".history",
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:    lokiAddressFlag,
			Aliases: []string{"loki"},
			EnvVars: []string{"APP_LOKI_ADDRESS"},
		}),
	}
	app := &cli.App{
		EnableBashCompletion: true,
		Name:                 "gcli",
		Before:               altsrc.InitInputSourceWithContext(flags, altsrc.NewYamlSourceFromFlagFunc("load")),
		Flags:                flags,
		Action:               runMain,
	}
	err := app.RunContext(context.Background(), os.Args)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Execution Time: %vs", time.Since(currentTime).Seconds())

}

var availableContentChan chan ListAvailableContentResponse
var retrievedContentObjects chan interface{}

var tracker *Tracker

func runMain(context *cli.Context) error {
	if context.Bool("debug") {
		log.Println("debug mode is enabled")
	}
	if context.Bool("AzureAD") {
		log.Println("AzureAD flag set to true")
	}
	if context.Bool("general") {
		log.Println("general flag set to true")
	}
	if context.Bool("Exchange") {
		log.Println("Exchange flag set to true")
	}
	if context.Bool("Sharepoint") {
		log.Println("sharepoint flag set to true")
	}
	if context.Bool("DLP") {
		log.Println("DLP flag set to true")
	}
	if staticLables := context.StringSlice("StaticLabel"); len(staticLables) > 0 {
		log.Printf("static labels set to: %v\n", staticLables)
	}
	if context.Bool("daemonize") {

		//cntxt := &daemon.Context{
		//	PidFileName: "opi.pid",
		//	PidFilePerm: 0644,
		//	LogFileName: "opi.log",
		//	LogFilePerm: 0640,
		//	WorkDir:     "/app",
		//	Args:        []string{"[o365_management_api]"},
		//}
		//defer func(cntxt *daemon.Context) {
		//	err := cntxt.Release()
		//	if err != nil {
		//		log.Fatal(err)
		//	}
		//}(cntxt)
		//d, err := cntxt.Reborn()
		//if err != nil {
		//	log.Fatal("unable to run: ", err)
		//}
		//if d != nil {
		//	return err
		//}
		log.Println("starting as daemon")
		runInterval := context.String("RunInterval")
		sleepDuration, err := time.ParseDuration(runInterval)
		if err != nil {
			log.Fatalf("Unable to parse duration value %v, run interval: %v", err, runInterval)
		}
		for {
			err = runFunc(context)
			if err != nil {
				log.Fatal("error", err)
				return err
			}
			log.Printf("Sleeping for %v", sleepDuration)
			time.Sleep(sleepDuration)
		}

	} else {
		return runFunc(context)

	}

}

func runFunc(context *cli.Context) error {

	tracker = &Tracker{
		hashSet:         hashmap.HashMap{},
		historyFilePath: context.String("HistoryFile"),
	}
	tracker.load()
	staticLabels := map[string]string{}
	for _, staticLabel := range context.StringSlice("StaticLabel") {
		k, v, err := splitStringOnChar(staticLabel, '=')
		if err != nil {
			log.Fatal(err)
		}
		staticLabels[k] = v
	}
	var wg sync.WaitGroup
	var loki promtail.Client
	if lokiAddress := context.String(lokiAddressFlag); lokiAddress != "" {
		conf := promtail.ClientConfig{
			PushURL:            lokiAddress,
			Labels:             staticLabels,
			SendLevel:          promtail.DEBUG,
			PrintLevel:         promtail.DISABLE,
			BatchWait:          time.Second * 5,
			BatchEntriesNumber: 500,
		}
		var err error
		loki, err = promtail.NewClientProto(conf)
		if err != nil {
			log.Fatal(fmt.Errorf("failed to generate promtail client from config: %w", err))
		}
	}

	// to manage request concurrency limit
	semaphorChan := make(chan struct{}, 20)

	//
	availableContentChan = make(chan ListAvailableContentResponse, MaxEntriesChanSize)

	// to hold responses
	retrievedContentObjects = make(chan interface{}, MaxEntriesChanSize)

	defer func(t *Tracker) {
		close(semaphorChan)
		close(availableContentChan)
		close(retrievedContentObjects)

		err := t.pruneHistory(time.Hour * 24 * 14)
		if err != nil {
			log.Println(err)
		}
	}(tracker)

	pubId = context.String("PublisherId")

	client, err := NewApiClientWithCustomEndpoint(TenantID, ApplicationID, ClientSecret, AzureADAuthEndpointGlobal, ServiceRootEndpointGlobal)
	if err != nil {
		log.Fatal(err)
	}
	err = client.refreshToken()
	if err != nil {
		return err
	}

	if context.Bool("General") {
		err := client.getContentForType(ContentType_General, context.Bool("debug"), &wg, context.Context)
		if err != nil {
			log.Fatal(err)
		}
	}
	if context.Bool("Exchange") {
		err := client.getContentForType(ContentType_Exchange, context.Bool("debug"), &wg, context.Context)
		if err != nil {
			log.Fatal(err)
		}
	}
	if context.Bool("Azure Ad") {
		err := client.getContentForType(ContentType_AAD, context.Bool("debug"), &wg, context.Context)
		if err != nil {
			log.Fatal(err)
		}
	}
	if context.Bool("Sharepoint") {
		err := client.getContentForType(ContentType_Sharepoint, context.Bool("debug"), &wg, context.Context)
		if err != nil {
			log.Fatal(err)
		}
	}
	if context.Bool("DLP") {
		err := client.getContentForType(ContentType_DLP, context.Bool("debug"), &wg, context.Context)
		if err != nil {
			log.Fatal(err)
		}
	}

	var outputFile *fileOutputWrapper

	if filePath := context.String("output_file"); filePath != "" {
		outputFile = &fileOutputWrapper{filePath: filePath}
		err := outputFile.open()
		if err != nil {
			return err
		}
		defer func(file *fileOutputWrapper) {
			err := file.close()
			if err != nil {

			}
		}(outputFile)
	}
loop:
	for {
		select {
		case result := <-retrievedContentObjects:
			wg.Add(1)
			go func(waitGroup *sync.WaitGroup, content interface{}, fileOutput *fileOutputWrapper, useLoki bool, lokiOutput promtail.Client) {
				defer waitGroup.Done()
				jsonObj, err := json.Marshal(&content)
				if len(jsonObj) == 0 {
					log.Printf("Encountered zero length marshalled json from object, possibly disposed before print?: %v\n", content)
					return
				} else if len(jsonObj) < 20 {
					log.Printf("Encountered zero length marshalled json from object, possibly disposed before print?: %v\n", content)
				}
				if err != nil {
					log.Print(err)
					return
				}
				if useLoki {
					//log.Println("sending to loki")
					if err != nil {
						log.Printf("error marshalling content object: %v", err)
					}
					lokiOutput.LogRaw(string(jsonObj), &map[string]string{}, promtail.INFO)
				}
				if fileOutput != nil {
					//log.Println("writing to outputFile")
					_, err := fileOutput.writeBytes(append([]byte{'\n'}, jsonObj...))
					if err != nil {
						log.Fatalf("failed to write to file: %v", err)
						return
					}
				}
			}(&wg, result, outputFile, "" != context.String(lokiAddressFlag), loki)
			//continue // we want to prioritize flushing out accumulated content

		case result := <-availableContentChan:
			if context.Bool("debug") {
				log.Printf("received content with uri %v from channel", result.ContentUri)
			}
			contentUri, err := url.ParseRequestURI(result.ContentUri)
			if err != nil {
				log.Printf("Error parsing request uri: %v", result.ContentUri)
				return err
			}
			wg.Add(1)
			go func(contentUri *url.URL, group *sync.WaitGroup, retrievedcontentChannel chan interface{}) {
				//var regOpts = compileListQueryOptions(nil)
				nextPageUri := contentUri.String()
				var err error
				var thisBatch []interface{}
				for {

					if err != nil {
						log.Fatal(err)

					}
					for _, retrievedContentObject := range thisBatch {
						retrievedcontentChannel <- &retrievedContentObject
					}

					if nextPageUri == "" {
						break
					}
					newUrl, err := url.Parse(nextPageUri)
					if err != nil {
						log.Printf("Error encountered attempting to parse: %v", nextPageUri)
					}
					// semephorChan for http request concurrency limiting
					semaphorChan <- struct{}{}
					//log.Printf("making request to uri: %v", newUrl.String())
					req, err := http.NewRequestWithContext(context.Context, http.MethodGet, newUrl.String(), nil)
					if err != nil {

						fmt.Printf("HTTP request error: %v", err)
					}

					// Deal with request Headers
					req.Header.Add("Content-Type", "application/json")
					req.Header.Add("Authorization", client.token.GetAccessToken())

					nextPageUri, err = client.performRequest(req, &thisBatch)
					<-semaphorChan
				}
				if err != nil {
					log.Println(err)
				}

				group.Done()
			}(contentUri, &wg, retrievedContentObjects)
			continue
		default:
			break
		}
		if len(retrievedContentObjects) == 0 && len(availableContentChan) == 0 && len(semaphorChan) == 0 {

			if len(retrievedContentObjects) == 0 && len(availableContentChan) == 0 {
				wg.Wait()
				break
			}

		}

	}

	if len(semaphorChan) > 0 || len(retrievedContentObjects) > 0 || len(availableContentChan) > 0 {
		goto loop
	}
	wg.Wait()
	if len(semaphorChan) > 0 || len(retrievedContentObjects) > 0 || len(availableContentChan) > 0 {
		goto loop
	}
	if "" != context.String(lokiAddressFlag) {
		loki.Shutdown()
	}
	return nil

}

func (g *ApiClient) getContentForType(contentType string, debug bool, waitGroup *sync.WaitGroup, ctx context.Context) error {
	for i := 0; i < chunkCount; i++ {
		waitGroup.Add(1)
		go func(wg *sync.WaitGroup, idx int, contentType string, ctime time.Time, debug bool, ctx context.Context, availcontentChan chan ListAvailableContentResponse) {
			// add 'token' to channel to keep track of concurrency (will block if concurrency limit is met
			//semaphorChan <- struct{}{}
			offset := time.Duration(chunkDuration.Nanoseconds() * (int64)(idx))
			startTime := ctime.Add(-offset).Add(-chunkDuration)
			endTime := ctime.Add(-offset)

			availContent, err := g.ListAvailableContent(startTime, endTime, contentType, ctx)
			if err != nil {
				log.Fatal(err)
			}
			for _, contentResponse := range availContent {
				if _, loaded := tracker.hashSet.GetOrInsert(contentResponse.ContentUri, currentTimeUnixString); !loaded {
					availcontentChan <- contentResponse
				} else {
					log.Printf("duplicate entry found %v \n", contentResponse.ContentUri)
				}
				// otherwise, it was already fetched
			}
			// remove token from semaphor to allow another to start
			//<-semaphorChan

			wg.Done()
		}(waitGroup, i, contentType, currentTime, debug, ctx, availableContentChan)
	}
	return nil
}
func (g *ApiClient) ListAvailableContent(startDateTime, endDateTime time.Time, contentType string, ctx context.Context, opts ...ListQueryOption) ([]ListAvailableContentResponse, error) {
	//resource := fmt.Sprintf("/subscriptions/content")//?contentType={ContentType}&amp;startTime={0}&amp;endTime={1}")
	//const resource = "subscriptions/content"
	var reqOpts = compileListQueryOptions(opts)
	const layout string = "2006-01-02T15:04:05"
	var cncFxn context.CancelFunc
	reqOpts.ctx, cncFxn = context.WithCancel(ctx)
	defer cncFxn()
	reqOpts.queryValues.Add("startTime", startDateTime.Format(layout))
	reqOpts.queryValues.Add("endTime", endDateTime.Format(layout))
	reqOpts.queryValues.Add("contentType", contentType)
	var availableContent []ListAvailableContentResponse
	//err := g.paginateAvailableContentRequest(&availableContent, reqOpts)
	//if err != nil {
	//	return nil, err
	//}
	var thisBatch []ListAvailableContentResponse
	nextPageUri, err := g.makeApiCall("subscriptions/content", http.MethodGet, reqOpts, nil, &thisBatch)

	for {
		if err != nil {
			log.Printf("Error encountered attempting to parse: %v", logStringSani(nextPageUri))
			return nil, err
		}
		availableContent = append(availableContent, thisBatch...)
		if nextPageUri == "" {
			break
		}
		newUrl, err := url.Parse(nextPageUri)
		if err != nil {
			log.Printf("Error encountered attempting to parse: %v", nextPageUri)
			return nil, err
		}
		req, err := http.NewRequestWithContext(reqOpts.Context(), http.MethodGet, newUrl.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("HTTP request error: %v", err)
		}

		// Deal with request Headers
		req.Header.Add("Content-Type", "application/json")
		req.Header.Add("Authorization", g.token.GetAccessToken())

		nextPageUri, err = g.performRequest(req, &thisBatch)
	}
	return availableContent, nil

}
