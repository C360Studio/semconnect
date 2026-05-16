# Upstream ask — Botts CS API ETS (`ets-ogcapi-connectedsystems10`)

**Repo:** <https://github.com/Botts-Innovative-Research/ets-ogcapi-connectedsystems10>
**Drafted from:** semconnect Stage 7 conformance run (2026-05-16), ETS pin `d9caf33`.
**Status:** ready to file (copy-paste).

## Summary

The `landingPageHasApiDefinitionLink` and `apiDefinitionResourceReturnsContent` tests enforce the presence of a `service-desc` / `service-doc` link on the landing page **unconditionally**, but OGC API Common Part 1 §7.4.1 Table 4 makes that link conditional on the server declaring either the `oas30` or `html` conformance class. Servers that legitimately implement Common Part 1 Core + JSON without OAS30 (because they do not ship an OpenAPI definition at this maturity level) cannot pass these tests, and per spec they should not be expected to.

This mirrors the pattern the suite already uses for `commonConformanceDeclaresCommonCore`, which checks `/conformance` first and then conditionally asserts.

## File / line refs (at ETS pin `d9caf33fcd0c4a3c1a582e8ba9b12b753277afd4`)

- `src/main/java/org/opengis/cite/ogcapiconnectedsystems10/conformance/core/LandingPageTests.java:172-183` — `landingPageHasApiDefinitionLink`; asserts unconditionally that `links[]` contains `rel=service-desc` or `rel=service-doc`. Constant `REQ_API_DEFINITION_SUCCESS = "http://www.opengis.net/spec/ogcapi-common-1/1.0/req/landing-page/api-definition-success"` defined at line 72.
- `src/main/java/org/opengis/cite/ogcapiconnectedsystems10/conformance/core/ResourceShapeTests.java:95-129` — `apiDefinitionResourceReturnsContent`; same unconditional shape, but the requirement constant is `REQ_OAS30_OAS_IMPL = "http://www.opengis.net/spec/ogcapi-common-1/1.0/req/oas30/oas-impl"` (line 56). The test self-identifies as an OAS30 verification yet runs against every IUT — strongest signal the conformance gate is missing.

## Existing helper to reuse

`src/main/java/org/opengis/cite/ogcapiconnectedsystems10/conformance/EncodingMediatypeWrite.java:61-65` already exposes a public static `declaresConformance(Map<String, Object> conformanceBody, String conformanceClass)` helper that parses `conformsTo[]` and returns whether `conformanceClass` is present. The proposed gate below uses it as-is — no new infrastructure needed.

Similar conditional-gate patterns already used by the suite:

- `EncodingMediatypeWrite.java:55` — `if (!declaresConformance(conformanceBody, conformanceClass))` → `SkipException`
- `UpdateTests.java:86-89`, `AdvancedFilteringTests.java:109-112` — fetch `/conformance`, check declaration, skip if absent.

## Observable impact

A v0.1 server that:

- ships `GET /` landing page with `self`, `conformance`, and `data` links (Common Part 1 §7.4 required set);
- declares `http://www.opengis.net/spec/ogcapi-common-1/1.0/conf/core` and `.../conf/json` in `/conformance`;
- does **not** declare `.../conf/oas30` because no OpenAPI definition is published;

…passes `landingPageReturnsHttp200`, `landingPageReturnsJson`, `commonLandingPageConformanceLinkHasJsonType`, `commonContentNegotiationHonoursFJsonParameter`, and `commonConformanceDeclaresCommonCore`, but fails the two tests above. The failure shape is:

```text
landingPageHasApiDefinitionLink:
  array did not contain any element matching ANY of: rel=service-desc link, rel=service-doc link (array size=4)

apiDefinitionResourceReturnsContent:
  neither rel=service-desc nor rel=service-doc present on landing page (api-definition fallback exhausted)
```

This blocks honest implementations from advancing through Common-Core conformance without either claiming OAS30 they don't implement (gaming) or shipping a stub OpenAPI definition with no real content (also gaming).

## Spec reference

OGC API Common Part 1 §7.4.1 Table 4 (link relations on the landing page):

| rel | Definition | Conditional |
|---|---|---|
| `self` | this resource | REQUIRED |
| `service-desc` | machine-readable API definition (OpenAPI 3.0) | REQUIRED if `oas30` or `html` conformance is declared, otherwise optional |
| `service-doc` | human-readable API definition (HTML) | same as `service-desc` |
| `conformance` | conformance declaration | REQUIRED |
| `data` | data resources | REQUIRED if any data is exposed |

(See <https://docs.ogc.org/is/19-072/19-072.html#_links>.)

## Proposed change

Gate both assertions on `declaresConformance(...)` using the existing helper. Either both `oas30` and `html` URIs trigger the assertion (matches the spec's "REQUIRED if oas30 OR html") or skip.

In `LandingPageTests.java` (around line 175):

```java
private static final String CONF_OAS30 =
    "http://www.opengis.net/spec/ogcapi-common-1/1.0/conf/oas30";
private static final String CONF_HTML =
    "http://www.opengis.net/spec/ogcapi-common-1/1.0/conf/html";

@Test(description = "OGC-19-072 " + REQ_API_DEFINITION_SUCCESS
        + ": landing page links contain rel=service-desc OR rel=service-doc "
        + "(REQ-ETS-CORE-002, SCENARIO-ETS-CORE-API-DEF-FALLBACK-001)",
        dependsOnMethods = "landingPageHasLinks", groups = "core")
public void landingPageHasApiDefinitionLink() {
    Map<String, Object> conformanceBody = fetchConformanceBody(); // already a helper or inline
    if (!EncodingMediatypeWrite.declaresConformance(conformanceBody, CONF_OAS30)
            && !EncodingMediatypeWrite.declaresConformance(conformanceBody, CONF_HTML)) {
        throw new SkipException(
            "Landing page api-definition link is conditional on oas30 or html "
            + "conformance per Common Part 1 §7.4.1 Table 4; server declares "
            + "neither, so this assertion is skipped per spec.");
    }
    // existing assertion body unchanged
}
```

`apiDefinitionResourceReturnsContent` in `ResourceShapeTests.java` needs the same guard at the top of its body (or `@Test(dependsOnMethods = "landingPageHasApiDefinitionLink")` if the class structure allows — TestNG propagates `SkipException` through `dependsOnMethods`).

## Backward-compat note

Servers that DO declare `oas30` or `html` see no behavior change — the existing assertion runs as before. Only servers that legitimately omit OAS30 + HTML start passing where they previously failed, which is exactly what the spec intends.

## Suggested triage

- **Interim** (no code change needed on either side): document the divergence in the ETS README and treat these two failures as "expected non-conformance for OAS30-less servers." Acceptable, but conformance dashboards will mis-report.
- **Ideal** (this proposal): land the `SkipException` guard, matching the pattern already used by `commonConformanceDeclaresCommonCore`. Small, surgical, restores spec-aligned behavior.
