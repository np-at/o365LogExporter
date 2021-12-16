package main

const (
	ContentType_General    = "Audit.General"
	ContentType_Exchange   = "Audit.Exchange"
	ContentType_Sharepoint = "Audit.Sharepoint"
	ContentType_AAD        = "Audit.AzureActiveDirectory"
	ContentType_DLP        = "DLP.All"

	ActivityLogUrlSpan = "activity/feed"
)
const (
	ApiVersion = "v1.0"
)

const (
	// AzureADAuthEndpointGlobal Azure AD authentication endpoint "Global". Used to aquire a token for the ms graph API connection.
	//
	// Microsoft Documentation: https://docs.microsoft.com/en-us/azure/active-directory/develop/authentication-national-cloud#azure-ad-authentication-endpoints
	AzureADAuthEndpointGlobal string = "https://login.microsoftonline.com"

	// OfficeManagementEndpointGlobalRoot Office Management API for enterprises
	//
	// see https://docs.microsoft.com/en-us/office/office-365-management-api/office-365-management-activity-api-reference
	OfficeManagementEndpointGlobalRoot = "https://manage.office.com/"

	// ServiceRootEndpointGlobal represents the default Service Root Endpoint used to perform all ms graph
	// API-calls, hence the Service Root Endpoint.
	//
	// See https://docs.microsoft.com/en-us/azure/active-directory/develop/authentication-national-cloud#azure-ad-authentication-endpoints
	ServiceRootEndpointGlobal string = "https://graph.microsoft.com"
)
