package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/urfave/cli/v2"
	"log"
	"net/http"
	"net/url"
	"o365_management_api/promtail-client/promtail"
	"os"
	"sync"
	"time"
)

const MAX_ENTRIES_CHAN_SIZE = 10000

var TenantID string      // See https://docs.microsoft.com/en-us/azure/azure-resource-manager/resource-group-create-service-principal-portal#get-tenant-id
var ApplicationID string // See https://docs.microsoft.com/en-us/azure/azure-resource-manager/resource-group-create-service-principal-portal#get-application-id-and-authentication-key
var ClientSecret string  //
var pubId string
var currentTime time.Time
var chunkDuration time.Duration
var chunkCount int

func main() {
	currentTime = time.Now().UTC()
	chunkDuration = time.Hour * 24
	chunkCount = 7

	app := &cli.App{

		Name: "gcli",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "TenantId",
				Destination: &TenantID,
				Aliases:     []string{"t"},
			},
			&cli.StringFlag{
				Name:        "ApplicationId",
				Aliases:     []string{"a"},
				Destination: &ApplicationID,
			},
			&cli.StringFlag{
				Name:        "ClientSecret",
				Aliases:     []string{"c"},
				Destination: &ClientSecret,
			},
			&cli.StringFlag{
				Name:    "output_file",
				Aliases: []string{"f"},
			},
			&cli.BoolFlag{
				Name: "General",
			},
			&cli.BoolFlag{
				Name:    "Exchange",
				Aliases: []string{"exchange"},
			},
			&cli.BoolFlag{
				Name:    "Azure Ad",
				Aliases: []string{"azuread"},
			},
			&cli.BoolFlag{
				Name: "Sharepoint",
			},
			&cli.BoolFlag{Name: "DLP"},
			&cli.BoolFlag{Name: "loki_output"},
			&cli.BoolFlag{
				Name:    "debug",
				Aliases: []string{"d"},
			},
			&cli.StringSliceFlag{
				Name: "PromtailLabel",
			},
		},
		Action: runFunc,
	}
	err := app.RunContext(context.Background(), os.Args)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Execution Time: %vs", time.Since(currentTime).Seconds())

}

func (g *ApiClient) getContentForType(contentType string, debug bool, waitGroup *sync.WaitGroup, ctx context.Context) error {
	for i := 0; i < chunkCount; i++ {
		waitGroup.Add(1)
		go func(wg *sync.WaitGroup, idx int, contentType string, ctime time.Time, debug bool, ctx context.Context) {
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
				availableCount <- struct{}{}
				availableContentChan <- &contentResponse
			}
			// remove token from semaphor to allow another to start
			//<-semaphorChan

			wg.Done()
		}(waitGroup, i, contentType, currentTime, debug, ctx)
	}
	return nil
}

var availableContentChan chan *ListAvailableContentResponse
var accumulatedContentChan chan *RetrievedContentObject

var collectedCount chan struct{}
var availableCount chan struct{}

func runFunc(context *cli.Context) error {
	var wg sync.WaitGroup
	conf := promtail.ClientConfig{
		PushURL:            "http://127.0.0.1:3100/loki/api/v1/push",
		Labels:             map[string]string{"job": "o365test2"},
		SendLevel:          promtail.DEBUG,
		PrintLevel:         promtail.DISABLE,
		BatchWait:          time.Second * 5,
		BatchEntriesNumber: 1000,
	}
	loki, err := promtail.NewClientProto(conf)

	collectedCount = make(chan struct{}, MAX_ENTRIES_CHAN_SIZE)
	availableCount = make(chan struct{}, MAX_ENTRIES_CHAN_SIZE)
	// to manage request concurrency limit
	semaphorChan := make(chan struct{}, 4)

	//
	availableContentChan = make(chan *ListAvailableContentResponse, MAX_ENTRIES_CHAN_SIZE)

	// to hold responses
	accumulatedContentChan = make(chan *RetrievedContentObject, MAX_ENTRIES_CHAN_SIZE)

	defer func() {
		close(availableCount)
		close(collectedCount)
		close(semaphorChan)
		close(availableContentChan)
		close(accumulatedContentChan)
	}()

	pubId = uuid.New().String()
	//pubId = ""

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

loop:
	for {
		select {
		case result := <-accumulatedContentChan:
			wg.Add(1)
			//if context.Bool("debug") {
			//	log.Printf("received content: %v from channel;  collectedREsult count at %v", result.Workload, collectedContentCount)
			//}

			go func(waitGroup *sync.WaitGroup, content *RetrievedContentObject, fileOutput string, useLoki bool, lokiOutput *promtail.Client) {
				defer waitGroup.Done()
				jsonObj, err := json.Marshal(*content)
				if err != nil {
					log.Print(err)
					return
				}
				if useLoki {
					//log.Println("sending to loki")
					if err != nil {
						log.Printf("error marshalling content object: %v", err)
					}
					(*lokiOutput).LogRaw(string(jsonObj), &map[string]string{})
				}
				if fileOutput != "" {
					//log.Println("writing to file")

					file, err := os.OpenFile(fileOutput, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0755)
					defer func(file *os.File) {
						err := file.Close()
						if err != nil {
							log.Print(err)
						}
					}(file)
					if err != nil {
						log.Print(err)
						return
					}
					_, err = file.Write([]byte{'\n'})
					if err != nil {
						log.Print(err)
						return
					}
					_, err = file.WriteString(string(jsonObj))
					if err != nil {
						log.Fatal(err)
					}
				}
				<-collectedCount
			}(&wg, result, context.String("output_file"), context.Bool("loki_output"), &loki)
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
			go func(contentUri *url.URL, group *sync.WaitGroup) {
				//var regOpts = compileListQueryOptions(nil)
				nextPageUri := contentUri.String()
				var err error
				var thisBatch []RetrievedContentObject
				for {

					if err != nil {
						log.Fatal(err)

					}
					for _, retrievedContentObject := range thisBatch {
						collectedCount <- struct{}{}
						accumulatedContentChan <- &retrievedContentObject
					}

					if nextPageUri == "" {
						break
					}
					newUrl, err := url.Parse(nextPageUri)
					if err != nil {
						log.Printf("Error encountered attempting to parse: %v", nextPageUri)
						log.Fatal(err)
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
			}(contentUri, &wg)
			<-availableCount
			continue
		default:
			break
		}
		if len(accumulatedContentChan) == 0 && len(availableContentChan) == 0 && len(semaphorChan) == 0 {
			if len(availableCount) > 0 {
				continue
			} else {
				if len(availableCount) > 0 || len(collectedCount) > 0 {
					continue
				}
				if len(accumulatedContentChan) == 0 && len(availableContentChan) == 0 {
					break
				}
			}

		}

	}

	if len(availableCount) > 0 || len(collectedCount) > 0 || len(semaphorChan) > 0 {
		goto loop
	}
	wg.Wait()
	if len(availableCount) > 0 || len(collectedCount) > 0 || len(semaphorChan) > 0 {
		goto loop
	}
	loki.Shutdown()
	return nil

}

type RetrievedContentObject struct {
	CreationTime                  string `json:"CreationTime"`
	Id                            string `json:"Id"`
	Operation                     string `json:"Operation"`
	OrganizationId                string `json:"OrganizationId"`
	RecordType                    int    `json:"RecordType"`
	ResultStatus                  string `json:"ResultStatus"`
	UserKey                       string `json:"UserKey"`
	UserType                      int    `json:"UserType"`
	Workload                      string `json:"Workload"`
	ClientIP                      string `json:"ClientIP,omitempty"`
	ObjectId                      string `json:"ObjectId"`
	UserId                        string `json:"UserId"`
	AzureActiveDirectoryEventType int    `json:"AzureActiveDirectoryEventType"`
	ExtendedProperties            []struct {
		Name  string `json:"Name"`
		Value string `json:"Value"`
	} `json:"ExtendedProperties,omitempty"`
	Client      string `json:"Client,omitempty"`
	LoginStatus int    `json:"LoginStatus,omitempty"`
	UserDomain  string `json:"UserDomain,omitempty"`
	Actor       []struct {
		ID   string `json:"ID"`
		Type int    `json:"Type"`
	} `json:"Actor,omitempty"`
	ActorContextId string `json:"ActorContextId,omitempty"`
	InterSystemsId string `json:"InterSystemsId,omitempty"`
	IntraSystemId  string `json:"IntraSystemId,omitempty"`
	Target         []struct {
		ID   string `json:"ID"`
		Type int    `json:"Type"`
	} `json:"Target,omitempty"`
	TargetContextId string `json:"TargetContextId,omitempty"`
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
			log.Printf("Error encountered attempting to parse: %v", nextPageUri)
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
