package csapi

import "github.com/c360studio/semstreams/message"

const (
	predDeploymentDeployedSystems      = "cs-api.deployment.deployedSystems"
	predSamplingFeatureHostedProcedure = "cs-api.samplingfeature.hostedProcedure"
)

func linksFromHrefs(hrefs []string, rel string) []link {
	links := make([]link, 0, len(hrefs))
	for _, href := range hrefs {
		if href == "" {
			continue
		}
		links = append(links, link{
			Href: href,
			Rel:  rel,
			Type: string(MediaJSON),
		})
	}
	return links
}

func firstLinkFromHref(href string, rel string) *link {
	if href == "" {
		return nil
	}
	return &link{
		Href: href,
		Rel:  rel,
		Type: string(MediaJSON),
	}
}

func triplesFromLinks(entityID string, predicate string, links []link) []message.Triple {
	triples := make([]message.Triple, 0, len(links))
	for _, l := range links {
		if l.Href == "" {
			continue
		}
		triples = append(triples, message.Triple{
			Subject:   entityID,
			Predicate: predicate,
			Object:    l.Href,
		})
	}
	return triples
}
