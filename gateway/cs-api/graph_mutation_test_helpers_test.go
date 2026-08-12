package csapi

import (
	"testing"

	"github.com/c360studio/semstreams/component"
)

var (
	SubjectEntityCreate    = mustTestGraphMutationSubject(graphMutationCreateOp)
	SubjectEntityReconcile = mustTestGraphMutationSubject(graphMutationReconcileOp)
	SubjectEntityDelete    = mustTestGraphMutationSubject(graphMutationDeleteOp)
)

func mustTestGraphMutationSubject(operation string) string {
	subject, err := component.ResolveSubject([]component.PortDefinition{{
		Name: graphMutationPortName,
		Config: component.NATSRequestPort{
			Subject: graphMutationSubjectFamily,
			Interface: &component.InterfaceContract{
				Type: graphMutationInterfaceType, Version: graphMutationInterfaceVer,
			},
		},
	}}, graphMutationPortName, operation)
	if err != nil {
		panic(err)
	}
	return subject
}

func requireMutationSubject(t *testing.T, component *Component, operation string) string {
	t.Helper()
	subject, err := component.graphMutationSubject(operation)
	if err != nil {
		t.Fatalf("resolve graph mutation subject %q: %v", operation, err)
	}
	return subject
}
