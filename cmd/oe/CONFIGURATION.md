# OE configuration reference

Every setting the Orchestration Engine reads. The overview and the short version are in the [README](README.md). Working examples: [config.example.json](config.example.json) (native run) and [config.docker.json](config.docker.json) (Docker Compose).

## Where configuration comes from

1. A JSON file at `CONFIG_PATH`, default `./config.json` relative to the working directory. The service exits at startup if the file is missing or invalid. `cmd/oe/config.json` is git-ignored.
2. A few environment variables (below). The OE does not load `.env` files.

`make run orchestration-engine` runs from `cmd/oe`, so `./config.json` is `cmd/oe/config.json`. In Compose the file is mounted at `/app/config.json`.

## config.json

| Key | Type | Notes |
|---|---|---|
| `foundationalIdArgName` | string | **Required.** The query argument that carries the data owner's ID, matched by name (for example `nic`). Startup fails if empty. If a query has none the OE returns `MISSING_IDENTIFIER`; if it has several different values, `AMBIGUOUS_IDENTIFIER`. |
| `pdpUrl` | string | Policy Decision Point base URL. The OE appends `/api/v1/policy/decide`. If empty, the PDP and consent checks are skipped. `pdpConfig.clientUrl` overrides it. |
| `ceUrl` | string | Consent Engine URL, which must end in `/internal/api/v1` because the OE appends `/consents`. `ceConfig.clientUrl` overrides it. If the PDP requires consent and this is empty, the request fails with `CE_ERROR`. |
| `providers` | array | Data providers; see below and [PROVIDER_CONFIGURATION.md](PROVIDER_CONFIGURATION.md). Loaded once at startup, so changes need a restart. |
| `argMappings` | array | Argument mappings; see below. |
| `auditConfig.serviceUrl` | string | Argus audit service URL. Overridden by `AUDIT_SERVICE_URL`. Auditing is off when this is empty. |
| `auditConfig.actorType`, `auditConfig.actorId` | string | Default `SERVICE` and `orchestration-engine`. |

Keys that are accepted but ignored: `server.port`, `log.level`, `services.pdp_url`, the removed schema keys `schema` and `sdl` (the schema comes from a file; see [Schema](#schema)), and the removed token settings `trustUpstream`, `jwt` and `environment`. Consumer tokens are configured with the `IDP_*` environment variables below.

### providers[]

| Key | Notes |
|---|---|
| `providerKey` | Unique name; must match `providerKey` in the schema's `@sourceInfo`. |
| `providerUrl` | The provider's GraphQL endpoint. The OE sends the sub-query as a `POST` to it. |
| `schemaId` | The provider schema's ID, as registered in Portal Backend and the PDP. `(providerKey, schemaId)` identifies a provider. |
| `auth.type` | `apiKey` or `oauth2`. Omit `auth` for an open provider. |
| `auth.apiKeyName`, `auth.apiKeyValue` | For `apiKey`: the header name and value sent with each request. |
| `auth.tokenUrl`, `auth.clientId`, `auth.clientSecret`, `auth.scopes` | For `oauth2`: client-credentials flow. `scopes` is a string array. |

### argMappings[]

Each entry maps an argument in the consumer's query to an argument of one provider's query.

| Key | Notes |
|---|---|
| `providerKey`, `schemaId` | The provider this mapping applies to. |
| `targetArgName` | The argument name the provider expects. |
| `sourceArgPath` | The consumer-side argument: a dash-separated path whose last segment is the argument name, for example `personInfo-nic`. |
| `targetArgPath` | Where the argument goes in the provider query: a leading part of the provider field path (the object the argument belongs to), for example `person` for the field `person.fullName`. |

## Environment variables

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `4000` | |
| `CONFIG_PATH` | `./config.json` | |
| `SCHEMA_PATH` | `./schema.graphql` | The consumer schema file. See [Schema](#schema). |
| `IDP_JWKS_URL` | `$IDP_BASE_URL/oauth2/jwks` | Consumer token signing keys. See [AUTHENTICATION.md](AUTHENTICATION.md#configuration). |
| `IDP_ISSUER` | `IDP_TOKEN_URL`, else `$IDP_BASE_URL/oauth2/token` | Expected token `iss`. |
| `IDP_AUDIENCE` | | **Required.** Expected token `aud`. |
| `IDP_JWKS_INSECURE_SKIP_VERIFY` | `false` | Skips TLS checks on the JWKS fetch. Local development only. |
| `AUDIT_SERVICE_URL` | | Overrides `auditConfig.serviceUrl`. |
| `ARGUS_API_KEY` | | Sent as `X-API-Key` to Argus. Falls back to `ARGUS_AUTH_TOKEN`. |
| `ENABLE_AUDIT` | on | Read by the audit client library. Audit is on when a URL is set and this is not `false`. |

[.env.example](.env.example) also lists `SERVICE_NAME`, `SERVICE_VERSION` and `OTEL_*`. The OE does not read them, and it does not serve `/metrics`. Compose passes some of them to every Go service.

## Schema

The OE reads the consumer GraphQL schema from the file at `SCHEMA_PATH` (default `./schema.graphql`, relative to the working directory) once at startup. It exits at startup if the file is missing or doesn't parse. In Compose the file is mounted at `/app/schema.graphql`.

To change the schema, deploy a new file and restart the OE: rebuild the image, or mount the file (for example from a Kubernetes ConfigMap) and roll out the pods. Every OE instance then serves the same schema. There is no API for changing the schema at runtime, and the OE uses no database.

Leaf fields in the schema must carry `@sourceInfo(providerKey, schemaId, providerField)`. See [schema.graphql](schema.graphql) for a sample.

## Error codes

GraphQL errors carry the code in `errors[].extensions.code`. Federation errors use HTTP 200.

| Code | Meaning |
|---|---|
| `PDP_NOT_ALLOWED` | The application is not authorized for some field, or its access has expired. `unauthorizedFields` or `expiredFields` lists them. |
| `PDP_ERROR` | The PDP call failed, or the PDP asked for consent but listed no fields. A field with no policy metadata causes this. |
| `PDP_NO_RESPONSE` | The PDP returned nothing. |
| `CE_NOT_APPROVED` | Consent is pending or otherwise not approved. `consentPortalUrl` and `consentStatus` tell the consumer where and what. |
| `CE_ERROR` | The Consent Engine call failed, or consent is needed and no Consent Engine is configured. |
| `CE_NO_RESPONSE` | The Consent Engine returned nothing. |
| `MISSING_IDENTIFIER` | The query has no `foundationalIdArgName` argument. |
| `AMBIGUOUS_IDENTIFIER` | The query has that argument with different values. |
| `BAD_REQUEST` | The query could not be parsed. |

Some errors carry only a `message` and no code, for example `Only query operation is supported` (mutations and subscriptions are rejected) and `No active schema found`.

HTTP-level errors: `400` for an unreadable request body, `401` with a JSON body such as `{"error":"unauthorized","message":"invalid authentication token"}` for a missing or invalid token.
