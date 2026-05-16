# Upstream ask — Botts CS API ETS (`ets-ogcapi-connectedsystems10`)

**Repo:** <https://github.com/Botts-Innovative-Research/ets-ogcapi-connectedsystems10>
**Drafted from:** semconnect Stage 7 conformance run (2026-05-16), ETS pin `d9caf33`.
**Status:** ready to file (copy-paste).

## Summary

The `landingPageHasApiDefinitionLink` and `apiDefinitionResourceReturnsContent` tests enforce the presence of a `service-desc` / `service-doc` link on the landing page **unconditionally**, but OGC API Common Part 1 §7.4.1 Table 4 makes that link conditional on the server declaring either the `oas30` or `html` conformance class. Servers that legitimately implement Common Part 1 Core + JSON without OAS30 (because they do not ship an OpenAPI definition at this maturity level) cannot pass these tests, and per spec they should not be expected to.

This mirrors the pattern the suite already uses for `commonConformanceDeclaresCommonCore`, which checks `/conformance` first and then conditionally asserts.

## File / line refs

- `src/main/java/.../landingpage/LandingPageTests.java` — `landingPageHasApiDefinitionLink` (assertion runs regardless of declared classes)
- `src/main/java/.../apidefinition/ApiDefinitionTests.java` — `apiDefinitionResourceReturnsContent` (cascades from above)

(Apologies — line numbers depend on the post-scaffold version; the assertion bodies should be findable by the test names above.)

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

Gate the `landingPageHasApiDefinitionLink` assertion on a precondition check:

```java
@Test(dependsOnMethods = "landingPageReturnsJson")
public void landingPageHasApiDefinitionLink() {
    if (!conformanceDeclaresAny(OAS30_URI, HTML_URI)) {
        throw new SkipException(
            "Landing page api-definition link is conditional on oas30 or html "
            + "conformance per Common Part 1 §7.4.1 Table 4; server declares "
            + "neither, so this assertion is skipped per spec.");
    }
    // existing assertion body
}
```

`apiDefinitionResourceReturnsContent` should chain off the same precondition (TestNG `@Test(dependsOnMethods = "landingPageHasApiDefinitionLink")` already handles the skip propagation).

## Backward-compat note

Servers that DO declare `oas30` or `html` see no behavior change — the existing assertion runs as before. Only servers that legitimately omit OAS30 + HTML start passing where they previously failed, which is exactly what the spec intends.

## Suggested triage

- **Interim** (no code change needed on either side): document the divergence in the ETS README and treat these two failures as "expected non-conformance for OAS30-less servers." Acceptable, but conformance dashboards will mis-report.
- **Ideal** (this proposal): land the `SkipException` guard, matching the pattern already used by `commonConformanceDeclaresCommonCore`. Small, surgical, restores spec-aligned behavior.
