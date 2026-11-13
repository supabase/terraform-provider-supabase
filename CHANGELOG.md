## 1.12.0 (Unreleased)

NOTES:

* Update Go to 1.26.11 ([#398](https://github.com/supabase/terraform-provider-supabase/pull/398))

## 1.11.1 (November 6, 2026)

NOTES:

* Bump Go to 1.26 and add a Go language policy ([#379](https://github.com/supabase/terraform-provider-supabase/pull/379))

ENHANCEMENTS:

* provider: Improve client telemetry headers ([#387](https://github.com/supabase/terraform-provider-supabase/pull/387))

BUG FIXES:

* resource/supabase_apikey: Include `description` when creating keys ([#361](https://github.com/supabase/terraform-provider-supabase/pull/361))
* resource/supabase_settings: Keep read-only storage keys out of state ([#388](https://github.com/supabase/terraform-provider-supabase/pull/388))

## 1.11.0 (September 2, 2026)

ENHANCEMENTS:

* provider: Validate and trim `SUPABASE_API_ENDPOINT` ([#352](https://github.com/supabase/terraform-provider-supabase/pull/352))

BUG FIXES:

* resource/supabase_settings: Forward every Postgres parameter in database settings ([#356](https://github.com/supabase/terraform-provider-supabase/pull/356))

## 1.10.1 (July 29, 2026)

FEATURES:

* **New Resource:** `supabase_third_party_auth` ([#334](https://github.com/supabase/terraform-provider-supabase/pull/334))

ENHANCEMENTS:

* resource/supabase_settings: Add `ssl_enforcement` support ([#320](https://github.com/supabase/terraform-provider-supabase/pull/320))
* resource/supabase_branch: Support declaring persistent branches ([#339](https://github.com/supabase/terraform-provider-supabase/pull/339))

BUG FIXES:

* resource/supabase_apikey: Enforce API key naming convention ([#322](https://github.com/supabase/terraform-provider-supabase/pull/322))
* resource/supabase_settings: Preserve Apple additional client IDs state ([#341](https://github.com/supabase/terraform-provider-supabase/pull/341))
* resource/supabase_branch: Fix default branch creation ([#338](https://github.com/supabase/terraform-provider-supabase/pull/338))
* resource/supabase_branch: Wait for branch project refs via the branches endpoint ([#348](https://github.com/supabase/terraform-provider-supabase/pull/348))

## 1.9.1 (May 15, 2026)

ENHANCEMENTS:

* resource/supabase_project: Add timeouts for create/update ([#278](https://github.com/supabase/terraform-provider-supabase/pull/278))
* resource/supabase_settings: Add timeouts for create/update ([#278](https://github.com/supabase/terraform-provider-supabase/pull/278))

BUG FIXES:

* resource/supabase_settings: Add external X secret to sensitive fields copied into state ([#312](https://github.com/supabase/terraform-provider-supabase/pull/312))
* resource/supabase_settings: Stop a disabled Data API from blocking changes ([#306](https://github.com/supabase/terraform-provider-supabase/pull/306))
* provider: Retry transient GET failures during `terraform refresh` ([#309](https://github.com/supabase/terraform-provider-supabase/pull/309))

## 1.9.0 (April 8, 2026)

FEATURES:

* **New Data Source:** `supabase_network_bans` ([#303](https://github.com/supabase/terraform-provider-supabase/pull/303))

## 1.8.2 (March 31, 2026)

FEATURES:

* **New Resource:** `supabase_edge_function_secrets` ([#298](https://github.com/supabase/terraform-provider-supabase/pull/298))

ENHANCEMENTS:

* provider: Add parameter validation and logging ([#290](https://github.com/supabase/terraform-provider-supabase/pull/290))
* resource/supabase_edge_function: Add import support with source download ([#285](https://github.com/supabase/terraform-provider-supabase/pull/285))
* resource/supabase_apikey: Support importing an API key by name ([#296](https://github.com/supabase/terraform-provider-supabase/pull/296))

BUG FIXES:

* provider: Trim whitespace from access token to prevent invalid header errors ([#292](https://github.com/supabase/terraform-provider-supabase/pull/292))
* provider: Fix token set in environment variable not being used ([#294](https://github.com/supabase/terraform-provider-supabase/pull/294))

## 1.7.0 (February 12, 2026)

FEATURES:

* **New Resource:** `supabase_edge_function` ([#270](https://github.com/supabase/terraform-provider-supabase/pull/270))

ENHANCEMENTS:

* resource/supabase_project: Add support for toggling legacy API keys ([#271](https://github.com/supabase/terraform-provider-supabase/pull/271))

BUG FIXES:

* resource/supabase_settings: Preserve user config values when the API returns null ([#268](https://github.com/supabase/terraform-provider-supabase/pull/268))
* resource/supabase_settings: Apply settings after project create ([#272](https://github.com/supabase/terraform-provider-supabase/pull/272))

## 1.6.2 (January 29, 2026)

ENHANCEMENTS:

* resource/supabase_project: Allow changing the database password ([#266](https://github.com/supabase/terraform-provider-supabase/pull/266))
* resource/supabase_project: Allow updating the project name ([#267](https://github.com/supabase/terraform-provider-supabase/pull/267))

BUG FIXES:

* resource/supabase_project: Add retry to network settings update ([#262](https://github.com/supabase/terraform-provider-supabase/pull/262))
* resource/supabase_settings: Add retry to network settings update ([#262](https://github.com/supabase/terraform-provider-supabase/pull/262))

## 1.6.1 (December 11, 2025)

BUG FIXES:

* resource/supabase_project: Populate `organization_slug` in project create request ([#265](https://github.com/supabase/terraform-provider-supabase/pull/265))

## 1.6.0 (December 11, 2025)

FEATURES:

* **New Resource:** `supabase_apikey` ([#256](https://github.com/supabase/terraform-provider-supabase/pull/256))

ENHANCEMENTS:

* resource/supabase_project: Allow updating a project's instance size ([#255](https://github.com/supabase/terraform-provider-supabase/pull/255))
* resource/supabase_settings: Add storage settings support ([#260](https://github.com/supabase/terraform-provider-supabase/pull/260))

BUG FIXES:

* Fix low risk command injection in release action ([#230](https://github.com/supabase/terraform-provider-supabase/pull/230))
* resource/supabase_settings: Respect `lifecycle.ignore_changes` ([#258](https://github.com/supabase/terraform-provider-supabase/pull/258))
* resource/supabase_settings: Keep `smtp_pass` consistent in auth config ([#257](https://github.com/supabase/terraform-provider-supabase/pull/257))

## 1.5.1 (January 9, 2025)

BUG FIXES:

* data-source/supabase_apikeys: Rename project ref attribute for consistency ([#216](https://github.com/supabase/terraform-provider-supabase/pull/216))

## 1.5.0 (January 8, 2025)

FEATURES:

* **New Data Source:** `supabase_apikeys` ([#212](https://github.com/supabase/terraform-provider-supabase/pull/212))

BUG FIXES:

* resource/supabase_project: Update types to match published API types ([#214](https://github.com/supabase/terraform-provider-supabase/pull/214))

## 1.4.2 (November 2, 2024)

BUG FIXES:

* resource/supabase_settings: Override all sensitive auth config fields ([#187](https://github.com/supabase/terraform-provider-supabase/pull/187))

## 1.4.1 (September 17, 2024)

BUG FIXES:

* resource/supabase_settings: Ignore masked SMTP password in auth config ([#157](https://github.com/supabase/terraform-provider-supabase/pull/157))

## 1.4.0 (May 3, 2024)

ENHANCEMENTS:

* resource/supabase_project: Support configuring instance size ([#75](https://github.com/supabase/terraform-provider-supabase/pull/75))
* resource/supabase_settings: Support configuring network restrictions ([#76](https://github.com/supabase/terraform-provider-supabase/pull/76))

## 1.3.0 (April 5, 2024)

ENHANCEMENTS:

* resource/supabase_settings: Support configuring database settings ([#56](https://github.com/supabase/terraform-provider-supabase/pull/56))

## 1.2.0 (March 28, 2024)

FEATURES:

* **New Data Source:** `supabase_pooler` ([#54](https://github.com/supabase/terraform-provider-supabase/pull/54))

## 1.1.3 (March 1, 2024)

BUG FIXES:

* Fix workflow permission for publishing draft release

## 1.1.2 (February 13, 2024)

BUG FIXES:

* data-source/supabase_branch: Return empty list when branching is disabled ([29c9f93](https://github.com/supabase/terraform-provider-supabase/commit/29c9f9316c8cc6c1f6932f75dffd0f2b3875a088))

## 1.1.1 (February 13, 2024)

BUG FIXES:

* data-source/supabase_branch: Return empty list when no branch exists ([ca68e2e](https://github.com/supabase/terraform-provider-supabase/commit/ca68e2e383e0aa00a7bc16cd92a2953e7c4f32a6))

## 1.1.0 (February 13, 2024)

FEATURES:

* **New Resource:** `supabase_branch` ([#19](https://github.com/supabase/terraform-provider-supabase/pull/19))

## 1.0.0 (February 8, 2024)

FEATURES:

* **New Data Source:** `supabase_branch` ([#2](https://github.com/supabase/terraform-provider-supabase/pull/2))
* **New Resource:** `supabase_project` ([#6](https://github.com/supabase/terraform-provider-supabase/pull/6))
* **New Resource:** `supabase_settings` ([#2](https://github.com/supabase/terraform-provider-supabase/pull/2))

ENHANCEMENTS:

* provider: Implement basic provider for project settings ([#2](https://github.com/supabase/terraform-provider-supabase/pull/2))
* resource/supabase_settings: Support auth config ([#16](https://github.com/supabase/terraform-provider-supabase/pull/16))
