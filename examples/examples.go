package examples

import _ "embed"

var (
	//go:embed resources/supabase_settings/resource.tf
	SettingsResourceConfig string
	//go:embed resources/supabase_project/resource.tf
	ProjectResourceConfig string
	//go:embed resources/supabase_branch/resource.tf
	BranchResourceConfig string
	//go:embed data-sources/supabase_branch/data-source.tf
	BranchDataSourceConfig string
	//go:embed data-sources/supabase_pooler/data-source.tf
	PoolerDataSourceConfig string
	//go:embed data-sources/supabase_apikeys/data-source.tf
	APIKeysDataSourceConfig string
)
