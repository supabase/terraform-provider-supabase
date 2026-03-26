package provider

const (
	testProjectRef             = "mayuaycdtijbctgqbycg" //nolint:gosec
	projectsApiPath            = "/v1/projects"
	projectApiPath             = projectsApiPath + "/" + testProjectRef
	apiKeysApiPath             = projectApiPath + "/api-keys"
	healthApiPath              = projectApiPath + "/health"
	branchesApiPath            = projectApiPath + "/branches"
	functionsApiPath           = projectApiPath + "/functions"
	poolerApiPath              = projectApiPath + "/config/database/pooler"
	billingApiPath             = projectApiPath + "/billing/addons"
	dbPasswordApiPath          = projectApiPath + "/database/password"
	dbConfigApiPath            = projectApiPath + "/config/database/postgres"
	networkBansApiPath         = projectApiPath + "/network-bans/retrieve"
	networkRestrictionsApiPath = projectApiPath + "/network-restrictions"
	postgrestApiPath           = projectApiPath + "/postgrest"
	authConfigApiPath          = projectApiPath + "/config/auth"
	storageConfigApiPath       = projectApiPath + "/config/storage"
	secretsApiPath             = projectApiPath + "/secrets"

	functionSlug          = "foo"
	testApiKeyUUID        = "d9bece6b-52cc-4d67-a948-2349d46676f5" //nolint:gosec
	testBranchUUID        = "3574ed44-5151-4f01-a6e3-2bc0339152d9" //nolint:gosec
	apiKeyApiPath         = apiKeysApiPath + "/" + testApiKeyUUID
	legacyApiKeysApiPath  = apiKeysApiPath + "/legacy"
	branchApiPath         = "/v1/branches/" + testBranchUUID
	deployFunctionApiPath = functionsApiPath + "/deploy"
	functionApiPath       = functionsApiPath + "/" + functionSlug
)
