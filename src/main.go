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

import "github.com/heptiolabs/healthcheck"

const MaxEntriesChanSize = 10000

const (
	debugFlag          = "debug"
	loadConfigFileFlag = "load"
	runAsDaemonFlag    = "daemonize"
	runIntervalFlag    = "RunInterval"
	outputFileFlag     = "output_file"
)

const lokiAddressFlag = "LokiAddress"
const (
	clientSecretFlag  = "ClientSecret"
	tenantIdFlag      = "TenantId"
	applicationIdFlag = "ApplicationId"
	publisherIdFlag   = "PublisherId"
)

const (
	historyFileFlag = "HistoryFile"
	jmesLabelsFlag  = "JMESLabels"
	staticLabelFlag = "StaticLabel"
)

const (
	getSharepointContentFlag = "Sharepoint"
	getAzureAdContentFlag    = "Azure AD"
	getGeneralContentFlag    = "General"
	getExchangeContentFlag   = "Exchange"
	getDLPContentFlag        = "DLP"
)

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
			Name:      loadConfigFileFlag,
			TakesFile: true,
		},
		&cli.BoolFlag{
			Name:    runAsDaemonFlag,
			Aliases: []string{"z"},
		},
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:    runIntervalFlag,
			Value:   "5m",
			EnvVars: []string{"APP_RUN_INTERVAL"},
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        tenantIdFlag,
			Destination: &TenantID,
			Aliases:     []string{"t"},
			EnvVars:     []string{"APP_TENANT_ID"},
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        applicationIdFlag,
			Aliases:     []string{"a"},
			Destination: &ApplicationID,
			EnvVars:     []string{"APP_APPLICATION_ID"},
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:        clientSecretFlag,
			Aliases:     []string{"c"},
			Destination: &ClientSecret,
			EnvVars:     []string{"APP_CLIENT_SECRET"},
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:      outputFileFlag,
			Aliases:   []string{"f"},
			TakesFile: true,
			Required:  false,
			EnvVars:   []string{"APP_OUTPUT_FILE"},
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:     publisherIdFlag,
			Required: false,
			EnvVars:  []string{"APP_PUBLISHER_ID"},
		}),
		altsrc.NewBoolFlag(&cli.BoolFlag{
			Name:    getGeneralContentFlag,
			EnvVars: []string{"APP_GENERAL"},
		}),

		altsrc.NewBoolFlag(&cli.BoolFlag{
			Name:    getExchangeContentFlag,
			Aliases: []string{"exchange"},
			EnvVars: []string{"APP_EXCHANGE"},
		}),
		altsrc.NewBoolFlag(&cli.BoolFlag{
			Name:    getAzureAdContentFlag,
			Aliases: []string{"azuread"},
			EnvVars: []string{"APP_AZUREAD"},
		}),
		altsrc.NewBoolFlag(&cli.BoolFlag{
			Name:    getSharepointContentFlag,
			EnvVars: []string{"APP_SHAREPOINT"},
		}),
		altsrc.NewBoolFlag(&cli.BoolFlag{
			Name:    getDLPContentFlag,
			EnvVars: []string{"APP_DLP"}}),

		altsrc.NewBoolFlag(&cli.BoolFlag{
			Name:    debugFlag,
			Aliases: []string{"d"},
			EnvVars: []string{"APP_DEBUG"},
		}),
		altsrc.NewStringSliceFlag(&cli.StringSliceFlag{
			Name:    staticLabelFlag,
			Aliases: []string{"l"},
			EnvVars: []string{"APP_STATIC_LABELS"},
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:      historyFileFlag,
			TakesFile: true,
			EnvVars:   []string{"APP_HISTORY_FILE"},
			Value:     ".history",
		}),
		altsrc.NewStringFlag(&cli.StringFlag{
			Name:    lokiAddressFlag,
			Aliases: []string{"loki"},
			EnvVars: []string{"APP_LOKI_ADDRESS"},
		}),
		altsrc.NewStringSliceFlag(&cli.StringSliceFlag{
			Name:    jmesLabelsFlag,
			EnvVars: []string{"APP_JMES_LABELS"},
		}),
	}
	app := &cli.App{
		EnableBashCompletion: true,
		Name:                 "gcli",
		Before:               altsrc.InitInputSourceWithContext(flags, altsrc.NewYamlSourceFromFlagFunc(loadConfigFileFlag)),
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
var jmesLabels = map[string]string{}

func runMain(context *cli.Context) error {

	if context.Bool(debugFlag) {
		log.Println("debug mode is enabled")
		for _, flag := range context.FlagNames() {
			if flagValue := context.Value(flag); flagValue == nil {
				log.Printf("%v present;\n", flag)
			} else {
				log.Printf("%v = %v;\n", flag, flagValue)
			}
		}
	}

	if staticLabels := context.StringSlice(staticLabelFlag); len(staticLabels) > 0 {
		log.Printf("static labels set to: %v\n", staticLabels)
	}
	if dynamicLabels := context.StringSlice(jmesLabelsFlag); len(dynamicLabels) > 0 {
		log.Printf("JMESPath labels set to: %v", dynamicLabels)
		for _, label := range dynamicLabels {
			k, v, err := splitStringOnChar(label, '=')
			if err != nil {
				log.Fatal(err)
			}
			jmesLabels[k] = v
		}
	}

	if context.Bool(runAsDaemonFlag) {
		log.Println("starting as daemon")
		health := healthcheck.NewHandler()

		health.AddLivenessCheck("goroutine-threshold", healthcheck.GoroutineCountCheck(100))
		health.AddLivenessCheck("gc-timeout", healthcheck.GCMaxPauseCheck(time.Second*3))
		go func() {
			err := http.ListenAndServe("0.0.0.0:8090", health)
			if err != nil {
				log.Fatalf("Error creating healthcheck http endpoint: %v", err)
			}
		}()
		sleepDuration, err := time.ParseDuration(context.String(runIntervalFlag))

		if err != nil {
			log.Fatalf("Unable to parse duration value %v, run interval: %v", err, sleepDuration)
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
		historyFilePath: context.String(historyFileFlag),
	}
	tracker.load()
	staticLabels := map[string]string{}
	for _, staticLabel := range context.StringSlice(staticLabelFlag) {
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

	pubId = context.String(publisherIdFlag)

	client, err := NewApiClientWithCustomEndpoint(TenantID, ApplicationID, ClientSecret, AzureADAuthEndpointGlobal, ServiceRootEndpointGlobal)
	if err != nil {
		log.Fatal(err)
	}
	err = client.refreshToken()
	if err != nil {
		return err
	}

	if context.Bool(getGeneralContentFlag) {
		err := client.getContentForType(ContentType_General, context.Bool(debugFlag), &wg, context.Context)
		if err != nil {
			log.Fatal(err)
		}
	}
	if context.Bool(getExchangeContentFlag) {
		err := client.getContentForType(ContentType_Exchange, context.Bool(debugFlag), &wg, context.Context)
		if err != nil {
			log.Fatal(err)
		}
	}
	if context.Bool(getAzureAdContentFlag) {
		err := client.getContentForType(ContentType_AAD, context.Bool(debugFlag), &wg, context.Context)
		if err != nil {
			log.Fatal(err)
		}
	}
	if context.Bool(getSharepointContentFlag) {
		err := client.getContentForType(ContentType_Sharepoint, context.Bool(debugFlag), &wg, context.Context)
		if err != nil {
			log.Fatal(err)
		}
	}
	if context.Bool(getDLPContentFlag) {
		err := client.getContentForType(ContentType_DLP, context.Bool(debugFlag), &wg, context.Context)
		if err != nil {
			log.Fatal(err)
		}
	}

	var outputFile *fileOutputWrapper

	if filePath := context.String(outputFileFlag); filePath != "" {
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
			go processRetrievedObject(&wg, result, outputFile, "" != context.String(lokiAddressFlag), loki)
		case result := <-availableContentChan:
			if context.Bool(debugFlag) {
				log.Printf("received content with uri %v from channel", result.ContentUri)
			}
			contentUri, err := url.ParseRequestURI(result.ContentUri)
			if err != nil {
				log.Printf("Error parsing request uri: %v", result.ContentUri)
				return err
			}
			wg.Add(1)
			go processAvailableObject(contentUri, &wg, retrievedContentObjects, semaphorChan, client, context)
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
func processAvailableObject(contentUri *url.URL, group *sync.WaitGroup, retrievedcontentChannel chan interface{}, semephorChan chan struct{}, client *ApiClient, cliContext *cli.Context) {
	//var regOpts = compileListQueryOptions(nil)
	nextPageUri := contentUri.String()
	var err error
	var thisBatch []map[string]interface{}
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
		semephorChan <- struct{}{}
		//log.Printf("making request to uri: %v", newUrl.String())
		req, err := http.NewRequestWithContext(cliContext.Context, http.MethodGet, newUrl.String(), nil)
		if err != nil {
			fmt.Printf("HTTP request error: %v", err)
		}
		// Deal with request Headers
		req.Header.Add("Content-Type", "application/json")
		req.Header.Add("Authorization", client.token.GetAccessToken())

		nextPageUri, err = client.performRequest(req, &thisBatch)
		<-semephorChan
	}
	if err != nil {
		log.Println(err)
	}

	group.Done()
}
func processRetrievedObject(waitGroup *sync.WaitGroup, content interface{}, fileOutput *fileOutputWrapper, useLoki bool, lokiOutput promtail.Client) {
	defer waitGroup.Done()
	var labels = map[string]string{}

	jsonObj, err := json.Marshal(&content)
	if err != nil {
		log.Println(err)
	}
	err = extractJMESLabels(jsonObj, &jmesLabels, &labels)
	if err != nil {
		log.Println(err)
	}

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
		lokiOutput.LogRaw(string(jsonObj), labels, promtail.INFO)
	}
	if fileOutput != nil {
		//log.Println("writing to outputFile")
		_, err := fileOutput.writeBytes(append([]byte{'\n'}, jsonObj...))
		if err != nil {
			log.Fatalf("failed to write to file: %v", err)
			return
		}
	}
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
