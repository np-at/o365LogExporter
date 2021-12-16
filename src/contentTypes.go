package main

type RetrievedContentObject struct {
	CreationTime                  string `json:"CreationTime"`
	Id                            string `json:"Id"`
	Operation                     string `json:"Operation"`
	OrganizationId                string `json:"OrganizationId"`
	RecordType                    int    `json:"RecordType,omitempty"`
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
	Scope           int    `json:"AuditLogScope,omitempty"`
}

type AADWorkloadResponse struct {
	CreationTime                  string `json:"CreationTime"`
	Id                            string `json:"Id"`
	Operation                     string `json:"Operation"`
	OrganizationId                string `json:"OrganizationId"`
	RecordType                    int    `json:"RecordType"`
	ResultStatus                  string `json:"ResultStatus,omitempty"`
	UserKey                       string `json:"UserKey"`
	UserType                      int    `json:"UserType"`
	Version                       int    `json:"Version"`
	Workload                      string `json:"Workload"`
	ClientIP                      string `json:"ClientIP"`
	ObjectId                      string `json:"ObjectId"`
	UserId                        string `json:"UserId"`
	AzureActiveDirectoryEventType int    `json:"AzureActiveDirectoryEventType"`
	ExtendedProperties            []struct {
		Name  string `json:"Name"`
		Value string `json:"Value"`
	} `json:"ExtendedProperties"`
	ModifiedProperties []interface{} `json:"ModifiedProperties"`
	Actor              []struct {
		ID   string `json:"ID"`
		Type int    `json:"Type"`
	} `json:"Actor"`
	ActorContextId  string `json:"ActorContextId"`
	ActorIpAddress  string `json:"ActorIpAddress"`
	InterSystemsId  string `json:"InterSystemsId"`
	IntraSystemId   string `json:"IntraSystemId"`
	SupportTicketId string `json:"SupportTicketId"`
	Target          []struct {
		ID   string `json:"ID"`
		Type int    `json:"Type"`
	} `json:"Target"`
	TargetContextId  string `json:"TargetContextId"`
	ApplicationId    string `json:"ApplicationId"`
	DeviceProperties []struct {
		Name  string `json:"Name"`
		Value string `json:"Value"`
	} `json:"DeviceProperties"`
	ErrorNumber string `json:"ErrorNumber"`
}
