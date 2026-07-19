package main

import (
	"strings"
	"testing"
)

func TestValidateGreenfieldState(t *testing.T) {
	t.Parallel()
	zero := 0
	one := 1
	two := 2
	ten := 10
	serverID := "NTEST"
	globalAccount := []accountDetail{{Name: "$G", ID: "$G"}}
	for _, tc := range []struct {
		name    string
		state   jetStreamResponse
		wantErr string
	}{
		{
			name: "clean",
			state: jetStreamResponse{
				ServerID: &serverID, Accounts: &one, Streams: &zero, Consumers: &zero,
				Messages: &zero, Bytes: &zero, AccountDetails: &globalAccount,
			},
		},
		{
			name:    "ten streams is not mistaken for zero",
			state:   validState(&serverID, &one, &ten, &zero, &globalAccount),
			wantErr: "streams=10",
		},
		{
			name:    "messages without streams fail closed",
			state:   validState(&serverID, &one, nil, &zero, &globalAccount),
			wantErr: "streams count is absent",
		},
		{
			name:    "unexpected account count",
			state:   validState(&serverID, &two, &zero, &zero, &globalAccount),
			wantErr: "account count 2",
		},
		{name: "missing server id", state: validState(nil, &one, &zero, &zero, &globalAccount), wantErr: "server_id"},
		{name: "missing account details", state: validState(&serverID, &one, &zero, &zero, nil), wantErr: "account_details"},
		{
			name: "wrong account identity",
			state: validState(
				&serverID, &one, &zero, &zero, &[]accountDetail{{Name: "APP", ID: "APP"}},
			),
			wantErr: "not exactly $G",
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := validateGreenfieldState(tc.state)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("validateGreenfieldState() error = %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("validateGreenfieldState() error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func validState(
	serverID *string,
	accounts *int,
	streams *int,
	zero *int,
	accountDetails *[]accountDetail,
) jetStreamResponse {
	return jetStreamResponse{
		ServerID: serverID, Accounts: accounts, Streams: streams, Consumers: zero,
		Messages: zero, Bytes: zero, AccountDetails: accountDetails,
	}
}

func TestValidateGreenfieldStateRequiresEveryCount(t *testing.T) {
	t.Parallel()
	zero := 0
	one := 1
	serverID := "NTEST"
	globalAccount := []accountDetail{{Name: "$G", ID: "$G"}}
	for _, field := range []string{"accounts", "streams", "consumers", "messages", "bytes"} {
		field := field
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			state := jetStreamResponse{
				ServerID: &serverID, Accounts: &one, Streams: &zero, Consumers: &zero,
				Messages: &zero, Bytes: &zero, AccountDetails: &globalAccount,
			}
			switch field {
			case "accounts":
				state.Accounts = nil
			case "streams":
				state.Streams = nil
			case "consumers":
				state.Consumers = nil
			case "messages":
				state.Messages = nil
			case "bytes":
				state.Bytes = nil
			}
			_, err := validateGreenfieldState(state)
			if err == nil || !strings.Contains(err.Error(), field) {
				t.Fatalf("validateGreenfieldState() error = %v, want missing %s", err, field)
			}
		})
	}
}
