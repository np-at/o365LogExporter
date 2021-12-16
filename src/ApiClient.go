package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

type ListAvailableContentResponse struct {
	ContentType       string `json:"contentType,omitempty"`
	ContentId         string `json:"contentId,omitempty"`
	ContentUri        string `json:"contentUri,omitempty"`
	ContentCreated    string `json:"contentCreated,omitempty"`
	ContentExpiration string `json:"contentExpiration,omitempty"`
}
type ApiClient struct {
	apiCall sync.Mutex

	TenantID      string // See https://docs.microsoft.com/en-us/azure/azure-resource-manager/resource-group-create-service-principal-portal#get-tenant-id
	ApplicationID string // See https://docs.microsoft.com/en-us/azure/azure-resource-manager/resource-group-create-service-principal-portal#get-application-id-and-authentication-key
	ClientSecret  string // See https://docs.microsoft.com/en-us/azure/azure-resource-manager/resource-group-create-service-principal-portal#get-application-id-and-authentication-key

	token Token // the current token to be used

	// azureADAuthEndpoint is used for this instance of ApiClient. For available endpoints see https://docs.microsoft.com/en-us/azure/active-directory/develop/authentication-national-cloud#azure-ad-authentication-endpoints
	azureADAuthEndpoint string
	// serviceRootEndpoint is the basic API-url used for this instance of ApiClient, namely Microsoft Graph service root endpoints. For available endpoints see https://docs.microsoft.com/en-us/graph/deployments#microsoft-graph-and-graph-explorer-service-root-endpoints.
	serviceRootEndpoint string
	// officeManageRootEndpoint
	officeManageRootEndpoint string

	publisherID string
}

func (g *ApiClient) String() string {
	var firstPart, lastPart string
	if len(g.ClientSecret) > 4 { // if ClientSecret is not initialized prevent a panic slice out of bounds
		firstPart = g.ClientSecret[0:3]
		lastPart = g.ClientSecret[len(g.ClientSecret)-3:]
	}
	return fmt.Sprintf("ApiClient(TenantID: %v, ApplicationID: %v, ClientSecret: %v...%v, Token validity: [%v - %v])",
		g.TenantID, g.ApplicationID, firstPart, lastPart, g.token.NotBefore, g.token.ExpiresOn)
}

func (g *ApiClient) makeApiCall(apiCall string, httpMethod string, reqParams getRequestParams, body io.Reader, v interface{}) (string, error) {
	g.makeSureURLsAreSet()
	g.apiCall.Lock()
	defer g.apiCall.Unlock()

	if g.token.WantsToBeRefreshed() {
		err := g.refreshToken()
		if err != nil {
			return "", err
		}
	}
	reqUrl, err := url.ParseRequestURI(g.officeManageRootEndpoint)
	if err != nil {
		return "", fmt.Errorf("unable to parse URI %v: %v", g.officeManageRootEndpoint, err)
	}

	reqUrl.Path = "/api/" + ApiVersion + "/" + g.TenantID + "/" + ActivityLogUrlSpan + "/" + apiCall
	req, err := http.NewRequestWithContext(reqParams.Context(), httpMethod, reqUrl.String(), body)
	if err != nil {
		return "", fmt.Errorf("HTTP request error: %v", err)
	}

	// Deal with request Headers
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", g.token.GetAccessToken())

	for key, vals := range reqParams.Headers() {
		for idx := range vals {
			req.Header.Add(key, vals[idx])
		}
	}

	// Deal with params
	var getParams = reqParams.Values()
	if pubId != "" {
		getParams.Add("PublisherIdentifier", pubId)
	}
	req.URL.RawQuery = getParams.Encode()
	return g.performRequest(req, v)
}

// NewApiClientWithCustomEndpoint NewApiClientCustomEndpoint creates a new ApiClient instance with the
// given parameters and tries to get a valid token. All available public endpoints
// for azureADAuthEndpoint and serviceRootEndpoint are available via msgraph.azureADAuthEndpoint*  and msgraph.ServiceRootEndpoint*
//
// For available endpoints from Microsoft, see documentation:
//   * Authentication Endpoints: https://docs.microsoft.com/en-us/azure/active-directory/develop/authentication-national-cloud#azure-ad-authentication-endpoints
//   * Service Root Endpoints: https://docs.microsoft.com/en-us/graph/deployments#microsoft-graph-and-graph-explorer-service-root-endpoints
//
// Returns an error if the token cannot be initialized. This func does not have
// to be used to create a new ApiClient.
func NewApiClientWithCustomEndpoint(tenantID, applicationID, clientSecret string, azureADAuthEndpoint string, serviceRootEndpoint string) (*ApiClient, error) {
	g := ApiClient{
		TenantID:            tenantID,
		ApplicationID:       applicationID,
		ClientSecret:        clientSecret,
		azureADAuthEndpoint: azureADAuthEndpoint,
		serviceRootEndpoint: serviceRootEndpoint,
		publisherID:         pubId,
	}
	if g.token.WantsToBeRefreshed() {
		g.apiCall.Lock()         // lock because we will refresh the token
		defer g.apiCall.Unlock() // unlock after token refresh
		return &g, g.refreshToken()
	}
	return &g, nil
}

// performRequestWithoutMarshal performs the http request but returns the response as []byte
func (g *ApiClient) performRequestForUnknown(req *http.Request, response *[]byte) (nextPageUri string, err error) {
	httpClient := &http.Client{
		Timeout: time.Second * 10,
	}

	resp, err := httpClient.Do(req)
	defer func(Body io.ReadCloser) {
		_ = Body.Close()

	}(resp.Body) // close body when func returns
	*response, err = ioutil.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// Hint: this will mostly be the case if the tenant ID cannot be found, the Application ID cannot be found or the clientSecret is incorrect.
		// The cause will be described in the body, hence we have to return the body too for proper error-analysis
		return "", fmt.Errorf("StatusCode is not OK: %v. Body: %v ", resp.StatusCode, string(*response))
	}
	nextPageUri = resp.Header.Get("NextPageUri")

	return nextPageUri, nil
}

// performRequest performs a pre-prepared http.Request and does the proper error-handling for it.
// does a json.Unmarshal into the v interface{} and returns the error of it if everything went well so far.
func (g *ApiClient) performRequest(req *http.Request, v interface{}) (string, error) {
	httpClient := &http.Client{
		Timeout: time.Second * 10,
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP response error: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()

	}(resp.Body) // close body when func returns

	body, err := ioutil.ReadAll(resp.Body) // read body first to append it to the error (if any)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// Hint: this will mostly be the case if the tenant ID cannot be found, the Application ID cannot be found or the clientSecret is incorrect.
		// The cause will be described in the body, hence we have to return the body too for proper error-analysis
		return "", fmt.Errorf("StatusCode is not OK: %v. Body: %v ", resp.StatusCode, string(body))
	}
	if err != nil {
		return "", fmt.Errorf("HTTP response read error: %w", err)
	}

	// no content returned when http PATCH or DELETE is used, e.g. User.DeleteUser()
	if req.Method == http.MethodDelete || req.Method == http.MethodPatch {
		return "", nil
	}
	//type skipTokenCallData struct {
	//	Data      []json.RawMessage `json:"value"`
	//	SkipToken string            `json:"@odata.nextLink"`
	//}
	//res := skipTokenCallData{}

	//err = json.Unmarshal(body, &res)
	//if err != nil {
	//	fmt.Printf("%v", string(body))
	//	return err
	//}
	//
	//if res.SkipToken == "" {
	//	return json.Unmarshal(body, &v) // return the error of the json unmarshal
	//}
	nextPageUri := resp.Header.Get("NextPageUri")

	return nextPageUri, json.Unmarshal(body, v)
	//data := res.Data
	//for res.SkipToken != "" {
	//	skipToken := res.SkipToken
	//	res = skipTokenCallData{}
	//	err := g.makeSkipTokenApiCall(req.Method, &res, skipToken)
	//	if err != nil {
	//		return err
	//	}
	//	data = append(data, res.Data...)
	//}
	//
	//var dataBytes []byte
	//
	////converts json.RawMessage into []bytes and adds a comma at the end
	//for _, v := range data {
	//	b, _ := v.MarshalJSON()
	//	dataBytes = append(dataBytes, b...)
	//	dataBytes = append(dataBytes, []byte(",")...)
	//}
	//
	//toReturn := []byte(`{"value":[`)                             //add missing "value" tag
	//toReturn = append(toReturn, dataBytes[:len(dataBytes)-1]...) //append previous data and skip last comma
	//toReturn = append(toReturn, []byte("]}")...)
	//
	//return json.Unmarshal(toReturn, &v) // return the error of the json unmarshal
}

// makeSureURLsAreSet ensures that the two fields g.azureADAuthEndpoint and g.serviceRootEndpoint
// of the graphClient are set and therefore not empty. If they are currently empty
// they will be set to the constants AzureADAuthEndpointGlobal and ServiceRootEndpointGlobal.
func (g *ApiClient) makeSureURLsAreSet() {
	if g.azureADAuthEndpoint == "" { // If AzureADAuthEndpoint is not set, use the global endpoint
		g.azureADAuthEndpoint = AzureADAuthEndpointGlobal
	}
	if g.serviceRootEndpoint == "" { // If ServiceRootEndpoint is not set, use the global endpoint
		g.serviceRootEndpoint = ServiceRootEndpointGlobal
	}
	if g.officeManageRootEndpoint == "" {
		g.officeManageRootEndpoint = OfficeManagementEndpointGlobalRoot
	}
}

// makeSkipTokenAPICall performs an API-Call to the msgraph API.
//
// Gets the results of the page specified by the skip token
func (g *ApiClient) makeSkipTokenApiCall(httpMethod string, v interface{}, skipToken string) error {

	// Check token
	if g.token.WantsToBeRefreshed() { // Token not valid anymore?
		err := g.refreshToken()
		if err != nil {
			return err
		}
	}

	req, err := http.NewRequest(httpMethod, skipToken, nil)
	if err != nil {
		return fmt.Errorf("HTTP request error: %v", err)
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", g.token.GetAccessToken())

	return g.performSkipTokenRequest(req, v)
}

// refreshToken refreshes the current Token. Grabs a new one and saves it within the ApiClient instance
func (g *ApiClient) refreshToken() error {
	g.makeSureURLsAreSet()
	if g.TenantID == "" {
		return fmt.Errorf("tenant ID is empty")
	}
	resource := fmt.Sprintf("/%v/oauth2/token", g.TenantID)
	data := url.Values{}
	data.Add("grant_type", "client_credentials")
	data.Add("client_id", g.ApplicationID)
	data.Add("client_secret", g.ClientSecret)
	data.Add("resource", g.officeManageRootEndpoint)

	u, err := url.ParseRequestURI(g.azureADAuthEndpoint)
	if err != nil {
		return fmt.Errorf("unable to parse URI: %v", err)
	}

	u.Path = resource
	req, err := http.NewRequest("POST", u.String(), bytes.NewBufferString(data.Encode()))

	if err != nil {
		return fmt.Errorf("HTTP Request Error: %v", logStringSani(err.Error()))
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Content-Length", strconv.Itoa(len(data.Encode())))

	var newToken Token
	_, err = g.performRequest(req, &newToken) // perform the prepared request
	if err != nil {
		return fmt.Errorf("error on getting msgraph Token: %v", err)
	}
	g.token = newToken
	return err
}

// performSkipTokenRequest performs a pre-prepared http.Request and does the proper error-handling for it.
// does a json.Unmarshal into the v interface{} and returns the error of it if everything went well so far.
func (g *ApiClient) performSkipTokenRequest(req *http.Request, v interface{}) error {
	httpClient := &http.Client{
		Timeout: time.Second * 10,
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP response error: %v of http.Request: %v", err, req.URL)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body) // close body when func returns

	body, err := ioutil.ReadAll(resp.Body) // read body first to append it to the error (if any)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// Hint: this will mostly be the case if the tenant ID cannot be found, the Application ID cannot be found or the clientSecret is incorrect.
		// The cause will be described in the body, hence we have to return the body too for proper error-analysis
		return fmt.Errorf("StatusCode is not OK: %v. Body: %v ", resp.StatusCode, string(body))
	}

	// fmt.Println("Body: ", string(body))

	if err != nil {
		return fmt.Errorf("HTTP response read error: %v of http.Request: %v", err, req.URL)
	}

	return json.Unmarshal(body, &v) // return the error of the json unmarshal
}
