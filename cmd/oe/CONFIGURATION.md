# OE configuration reference

Every setting the Orchestration Engine reads. The overview and the short version are in the [README](README.md). Working examples: [config.example.json](config.example.json) (native run) and [config.docker.json](config.docker.json) (Docker Compose).

## Where configuration comes from

1. A JSON file at `CONFIG_PATH`, default `./config.json` relative to the working directory. The service exits at startup if the file is missing or invalid. `cmd/oe/config.json` is git-ignored.
2. A few environment variables (below). The OE does not load `.env` files.

`make run orchestration-engine` runs from `cmd/oe`, so `./config.json` is `cmd/oe/config.json`. In Compose the file is mounted at `/app/config.json`.

## config.json

| Key | Type | Notes |
|---|---|---|
| `schemaPath` | string | The consumer schema file. Default `schema.graphql`; a relative path is resolved against the directory of `config.json`. See [Schema](#schema). |
| `foundationalIdArgName` | string | **Required.** The query argument that carries the data owner's ID, matched by name (for example `nic`) on every field of the query, aliases included. Startup fails if empty. If a query has none the OE returns `MISSING_IDENTIFIER`; if it has several different values, `AMBIGUOUS_IDENTIFIER`. |
| `pdpUrl` | string | Policy Decision Point base URL. The OE appends `/api/v1/policy/decide`. If empty, the PDP and consent checks are skipped. `pdpConfig.clientUrl` overrides it. |
| `ceUrl` | string | Consent Engine URL, which must end in `/internal/api/v1` because the OE appends `/consents`. `ceConfig.clientUrl` overrides it. If the PDP requires consent and this is empty, the request fails with `CE_ERROR`. |
| `providers` | array | Data providers; see below and [PROVIDER_CONFIGURATION.md](PROVIDER_CONFIGURATION.md). Loaded once at startup, so changes need a restart. |
| `argMappings` | array | Argument mappings; see below. |
| `auditConfig.serviceUrl` | string | Argus audit service URL. Overridden by `AUDIT_SERVICE_URL`. Auditing is off when this is empty. |
| `auditConfig.actorType`, `auditConfig.actorId` | string | Default `SERVICE` and `orchestration-engine`. |

Keys that are accepted but ignored: `server.port`, `log.level`, `services.pdp_url`, the removed inline schema keys `schema` and `sdl` (the schema comes from the `schemaPath` file), and the removed token settings `trustUpstream`, `jwt` and `environment`. Consumer tokens are configured with the `IDP_*` environment variables below.

### providers[]

| Key | Notes |
|---|---|
| `schemaId` | The provider schema's ID, as registered in Portal Backend and the PDP. Unique; identifies the provider and must match `schemaId` in the schema's `@sourceInfo`. |
| `providerUrl` | The provider's GraphQL endpoint. The OE sends the sub-query as a `POST` to it. |
| `auth.type` | `apiKey` or `oauth2`. Omit `auth` for an open provider. |
| `auth.apiKeyName`, `auth.apiKeyValue` | For `apiKey`: the header name and value sent with each request. |
| `auth.tokenUrl`, `auth.clientId`, `auth.clientSecret`, `auth.scopes` | For `oauth2`: client-credentials flow. `scopes` is a string array. |

### argMappings[]

Each entry maps an argument in the consumer's query to an argument of one provider's query.

| Key | Notes |
|---|---|
| `schemaId` | The provider this mapping applies to. |
| `targetArgName` | The argument name the provider expects. |
| `sourceArgPath` | The consumer-side argument: the consumer field path from the root, then the argument name, joined with dashes. `personInfo-nic` is the `nic` argument of the root field `personInfo`; an argument with the same name on another field doesn't match. |
| `targetArgPath` | The provider field the argument is attached to: a leading part of the provider field path, for example `person` for the field `person.fullName`. The argument is attached to that field only, not to the fields above it. |

A mapped argument is sent with every provider field under its `targetArgPath` that is selected below the consumer field carrying the argument. When aliases give the same mapped argument different values, the OE sends a separate provider query for each value.

## Environment variables

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `4000` | |
| `CONFIG_PATH` | `./config.json` | |
| `IDP_JWKS_URL` | `$IDP_BASE_URL/oauth2/jwks` | Consumer token signing keys. See [AUTHENTICATION.md](AUTHENTICATION.md#configuration). |
| `IDP_ISSUER` | `IDP_TOKEN_URL`, else `$IDP_BASE_URL/oauth2/token` | Expected token `iss`. |
| `IDP_AUDIENCE` | | **Required.** Expected token `aud`. |
| `IDP_JWKS_INSECURE_SKIP_VERIFY` | `false` | Skips TLS checks on the JWKS fetch. Local development only. |
| `AUDIT_SERVICE_URL` | | Overrides `auditConfig.serviceUrl`. |
| `ARGUS_API_KEY` | | Sent as `X-API-Key` to Argus. Falls back to `ARGUS_AUTH_TOKEN`. |
| `ENABLE_AUDIT` | on | Read by the audit client library. Audit is on when a URL is set and this is not `false`. |

[.env.example](.env.example) also lists `SERVICE_NAME`, `SERVICE_VERSION` and `OTEL_*`. The OE does not read them, and it does not serve `/metrics`. Compose passes some of them to every Go service.

## Schema

The OE reads the consumer GraphQL schema from the file named by `schemaPath` in `config.json` once at startup. The default is `schema.graphql`, and a relative path is resolved against the directory that contains `config.json`, so by default the schema sits next to the config. An absolute path is used as is. The OE exits at startup if the file is missing or isn't a valid schema (see below). In Compose both files are mounted in `/app`.

The image contains only the binary, so the schema, like `config.json`, is mounted into the container. To change the schema, mount the new file (for example from a Kubernetes ConfigMap) and restart the OE or roll out the pods. Every OE instance then serves the same schema. There is no API for changing the schema at runtime, and the OE uses no database.

At startup the OE validates the schema against the GraphQL spec (with [gqlparser](https://github.com/vektah/gqlparser)) and compiles it once.

Leaf fields in the schema must carry `@sourceInfo(schemaId, providerField)`, with both set. The schema may declare the directive itself; if it doesn't, the OE adds `directive @sourceInfo(schemaId: String!, providerField: String!) on FIELD_DEFINITION`. See [schema.graphql](schema.graphql) for a sample.

A list field with `@sourceInfo` points at a list in the provider's response, and its item fields are read from each item. An item field's `providerField` is either the full path, starting with the list's path (`vehicle.getVehicleInfos.data.make`), or just the field's name in the item (`make`).

Every consumer query is validated against the schema before the OE calls the PDP: unknown fields, wrong argument types and missing required arguments are rejected with `BAD_REQUEST`.

### Queries

The OE follows the GraphQL spec's field collection: fragments and inline fragments are merged, `@skip` and `@include` are applied, and the response uses the query's aliases. Only the provider fields a query actually selects are sent to the PDP and fetched, each provider once per set of mapped argument values. `__typename` is answered by the OE.

Not supported, and rejected with `BAD_REQUEST`:

- selections on fields of interface or union type, because the OE can't tell which type a provider returned;
- selections on a list field without `@sourceInfo`, because there is no provider list to read the items from;
- introspection (`__schema`, `__type`).

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
| `MISSING_IDENTIFIER` | The query has no value for the `foundationalIdArgName` argument. When the schema declares that argument as required (for example `nic: String!`), a query without it fails validation with `BAD_REQUEST` instead. |
| `AMBIGUOUS_IDENTIFIER` | The query has that argument with different values, for example on two aliased fields. The PDP isn't called. |
| `BAD_REQUEST` | The query could not be parsed, failed validation against the schema, has invalid variables, or uses something the OE doesn't support (see [Queries](#queries)). Parse and validation errors carry `locations` in the query. |
| `PROVIDER_ERROR` | A provider call failed: the provider was unreachable or not configured, answered with a non-2xx HTTP status, or sent a body that isn't a GraphQL response. Also used for errors a provider returned in its response, with the message prefixed by the provider's schema ID. |

Mutations and subscriptions are rejected: with `BAD_REQUEST` when the schema has no `Mutation` or `Subscription` type, otherwise with the message `Only query operation is supported` and no code.

A provider failure doesn't fail the whole query. The fields that provider serves are `null`, each with a `PROVIDER_ERROR` at its `path`, and the other providers' data is returned. As in the GraphQL spec, a `null` in a non-null field (`String!`) makes its parent `null` instead, up to the nearest field that can be `null`; the error then reads `Cannot return null for non-nullable field …`. If that reaches a non-null root field, `data` is `null`.

HTTP-level errors: `400` for an unreadable request body, `401` with a JSON body such as `{"error":"unauthorized","message":"invalid authentication token"}` for a missing or invalid token.
