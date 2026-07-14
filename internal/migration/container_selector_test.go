package migration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestDiscoverContainerFailsClosed(t *testing.T) {
	config := ResolvedConfig{Host: "postgres", Port: 5432, Database: "alice", Username: "guardian"}
	fullID := strings.Repeat("a", 64)
	base := ContainerDetails{ID: fullID, Image: "bitnami/postgresql:11-debian-10", State: ContainerRunning, Health: HealthNone, NetworkMode: "bridge", DatabaseNames: []string{"alice"}, Usernames: []string{"guardian"}, Endpoints: []ContainerEndpoint{{Host: "postgres", Port: 5432, ContainerLocal: true}}, MountKinds: []string{"bitnami-postgresql-data"}, Labels: SafeContainerLabels{ComposeService: "postgresql"}}

	for _, tt := range []struct {
		name       string
		candidates []ContainerSummary
		details    map[string]ContainerDetails
		want       error
	}{
		{"one corroborated running container", []ContainerSummary{{ID: fullID, Image: base.Image}}, map[string]ContainerDetails{fullID: base}, nil},
		{"image-only evidence is rejected", []ContainerSummary{{ID: fullID, Image: base.Image}}, map[string]ContainerDetails{fullID: {ID: fullID, Image: base.Image, State: ContainerRunning, Health: HealthNone}}, ErrContainerPrecondition},
		{"database mismatch is rejected", []ContainerSummary{{ID: fullID, Image: base.Image}}, map[string]ContainerDetails{fullID: func() ContainerDetails { d := base; d.DatabaseNames = []string{"other"}; return d }()}, ErrContainerPrecondition},
		{"user mismatch is rejected", []ContainerSummary{{ID: fullID, Image: base.Image}}, map[string]ContainerDetails{fullID: func() ContainerDetails { d := base; d.Usernames = []string{"other"}; return d }()}, ErrContainerPrecondition},
		{"stopped container is rejected", []ContainerSummary{{ID: fullID, Image: base.Image}}, map[string]ContainerDetails{fullID: func() ContainerDetails { d := base; d.State = ContainerStopped; return d }()}, ErrContainerPrecondition},
		{"unhealthy container is rejected", []ContainerSummary{{ID: fullID, Image: base.Image}}, map[string]ContainerDetails{fullID: func() ContainerDetails { d := base; d.Health = HealthUnhealthy; return d }()}, ErrContainerPrecondition},
		{"host endpoint cannot substitute container local endpoint", []ContainerSummary{{ID: fullID, Image: base.Image}}, map[string]ContainerDetails{fullID: func() ContainerDetails {
			d := base
			d.Endpoints = []ContainerEndpoint{{Host: "localhost", Port: 5432}}
			return d
		}()}, ErrContainerPrecondition},
		{"prefix ID is rejected", []ContainerSummary{{ID: fullID[:12], Image: base.Image}}, map[string]ContainerDetails{fullID[:12]: base}, ErrContainerPrecondition},
		{"multiple plausible candidates are ambiguous", []ContainerSummary{{ID: fullID, Image: base.Image}, {ID: strings.Repeat("b", 64), Image: base.Image}}, map[string]ContainerDetails{fullID: base, strings.Repeat("b", 64): func() ContainerDetails { d := base; d.ID = strings.Repeat("b", 64); return d }()}, ErrAmbiguousContainer},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DiscoverContainer(context.Background(), &fakeInspector{candidates: tt.candidates, details: tt.details}, config)
			if !errors.Is(err, tt.want) {
				t.Fatalf("DiscoverContainer() error = %v, want %v", err, tt.want)
			}
			if tt.want == nil && got.ID != fullID {
				t.Fatalf("selected ID = %q", got.ID)
			}
			assertNoDockerLeak(t, err, got)
		})
	}
}

func TestDiscoverContainerCollectsOnlySufficientlyCorroboratedCandidates(t *testing.T) {
	config := ResolvedConfig{Host: "postgres", Port: 5432, Database: "alice", Username: "guardian"}
	validID, unrelatedID := strings.Repeat("a", 64), strings.Repeat("b", 64)
	valid := ContainerDetails{ID: validID, Image: string(PostgreSQL11Image), State: ContainerRunning, Health: HealthNone, DatabaseNames: []string{"alice"}, Usernames: []string{"guardian"}, Endpoints: []ContainerEndpoint{{Host: "postgres", Port: 5432, ContainerLocal: true}}, MountKinds: []string{"bitnami-postgresql-data"}}

	for _, tt := range []struct {
		name       string
		candidates []ContainerSummary
		details    map[string]ContainerDetails
		wantID     string
		wantErr    error
	}{
		{"one valid plus one unrelated selects valid", []ContainerSummary{{ID: unrelatedID, Image: string(PostgreSQL11Image)}, {ID: validID, Image: string(PostgreSQL11Image)}}, map[string]ContainerDetails{validID: valid, unrelatedID: {ID: unrelatedID, Image: string(PostgreSQL11Image), State: ContainerRunning, Health: HealthNone}}, validID, nil},
		{"zero candidates is a precondition failure", nil, nil, "", ErrContainerPrecondition},
		{"conflicting endpoint evidence rejects the candidate", []ContainerSummary{{ID: validID, Image: string(PostgreSQL11Image)}}, map[string]ContainerDetails{validID: func() ContainerDetails {
			d := valid
			d.Endpoints = append(d.Endpoints, ContainerEndpoint{Host: "postgres", Port: 15432, ContainerLocal: true})
			return d
		}()}, "", ErrContainerPrecondition},
		{"allowed mount independently corroborates", []ContainerSummary{{ID: validID, Image: string(PostgreSQL11Image)}}, map[string]ContainerDetails{validID: valid}, validID, nil},
		{"allowed label independently corroborates", []ContainerSummary{{ID: validID, Image: string(PostgreSQL11Image)}}, map[string]ContainerDetails{validID: func() ContainerDetails {
			d := valid
			d.MountKinds = nil
			d.Labels = SafeContainerLabels{ComposeService: "postgresql"}
			return d
		}()}, validID, nil},
		{"unknown declared health is rejected", []ContainerSummary{{ID: validID, Image: string(PostgreSQL11Image)}}, map[string]ContainerDetails{validID: func() ContainerDetails { d := valid; d.Health = HealthUnknown; return d }()}, "", ErrContainerPrecondition},
		{"image alias is not exact normalized identity", []ContainerSummary{{ID: validID, Image: "docker.io/bitnami/postgresql:11-debian-10"}}, map[string]ContainerDetails{validID: valid}, "", ErrContainerPrecondition},
	} {
		t.Run(tt.name, func(t *testing.T) {
			inspector := &fakeInspector{candidates: tt.candidates, details: tt.details}
			got, err := DiscoverContainer(context.Background(), inspector, config)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("DiscoverContainer() error = %v, want %v", err, tt.wantErr)
			}
			if got.ID != tt.wantID {
				t.Fatalf("selected ID = %q, want %q", got.ID, tt.wantID)
			}
			if len(tt.candidates) == 2 && strings.Join(inspector.inspected, ",") != validID+","+unrelatedID {
				t.Fatalf("inspection order = %v", inspector.inspected)
			}
			assertNoDockerLeak(t, got, err)
		})
	}
}

func TestDockerCLIInspectorRedactsSensitiveMountData(t *testing.T) {
	id := strings.Repeat("a", 64)
	inspect := `[{"Id":"` + id + `","Config":{"Image":"bitnami/postgresql:11-debian-10","Labels":{}},"State":{"Running":true},"Mounts":[{"Name":"private-volume","Destination":"/private/synthetic-secret-must-not-escape"},{"Name":"bitnami-postgresql-data","Destination":"/bitnami/postgresql"}]}]`
	details, ok := safeDetails(dockerInspectFromJSON(t, inspect))
	if !ok || len(details.MountKinds) != 1 || details.MountKinds[0] != "bitnami-postgresql-data" {
		t.Fatalf("safeDetails() = %#v, %v", details, ok)
	}
	assertNoDockerLeak(t, details)
}

func dockerInspectFromJSON(t *testing.T, raw string) dockerInspect {
	t.Helper()
	var inspected []dockerInspect
	if err := json.Unmarshal([]byte(raw), &inspected); err != nil || len(inspected) != 1 {
		t.Fatalf("invalid test fixture: %v", err)
	}
	return inspected[0]
}

func TestDiscoverContainerInspectsEveryExactCandidateAndRejectsFailures(t *testing.T) {
	id1, id2 := strings.Repeat("a", 64), strings.Repeat("b", 64)
	config := ResolvedConfig{Host: "postgres", Port: 5432, Database: "alice", Username: "guardian"}
	inspector := &fakeInspector{candidates: []ContainerSummary{{ID: id1, Image: "bitnami/postgresql:11-debian-10"}, {ID: id2, Image: "bitnami/postgresql:11-debian-10"}}, details: map[string]ContainerDetails{id1: {ID: id1, Image: "bitnami/postgresql:11-debian-10"}, id2: {ID: id2, Image: "bitnami/postgresql:11-debian-10"}}, inspectErr: map[string]error{id2: errors.New("daemon denied: secret-env")}}
	_, err := DiscoverContainer(context.Background(), inspector, config)
	if !errors.Is(err, ErrContainerPrecondition) {
		t.Fatalf("error = %v", err)
	}
	if strings.Join(inspector.inspected, ",") != id1+","+id2 {
		t.Fatalf("inspected = %v", inspector.inspected)
	}
	assertNoDockerLeak(t, err)
}

func TestDockerCLIInspectorRejectsMalformedOrUnavailableDockerData(t *testing.T) {
	id := strings.Repeat("a", 64)
	for _, tt := range []struct {
		name    string
		runner  *fakeDockerRunner
		inspect bool
	}{
		{"daemon error", &fakeDockerRunner{errs: []error{errors.New("permission denied: synthetic-secret-must-not-escape")}}, false},
		{"malformed list", &fakeDockerRunner{outputs: [][]byte{[]byte("not-a-container\tbitnami/postgresql:11-debian-10\n")}}, false},
		{"malformed inspect", &fakeDockerRunner{outputs: [][]byte{[]byte(id + "\tbitnami/postgresql:11-debian-10\n"), []byte("not-json")}}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			inspector := DockerCLIInspector{Runner: tt.runner}
			if tt.inspect {
				_, _ = inspector.Candidates(context.Background(), PostgreSQL11Image)
				_, err := inspector.Inspect(context.Background(), id)
				if !errors.Is(err, ErrContainerPrecondition) {
					t.Fatalf("Inspect() error = %v", err)
				}
				return
			}
			_, err := inspector.Candidates(context.Background(), PostgreSQL11Image)
			if !errors.Is(err, ErrContainerPrecondition) {
				t.Fatalf("Candidates() error = %v", err)
			}
			assertNoDockerLeak(t, err)
		})
	}
}

func TestDockerCLIInspectorParsesOnlyAllowlistedMetadata(t *testing.T) {
	id := strings.Repeat("a", 64)
	inspect := `[{"Id":"` + id + `","Config":{"Image":"bitnami/postgresql:11-debian-10","Env":["POSTGRESQL_DATABASE=alice","POSTGRESQL_USERNAME=guardian","POSTGRESQL_PASSWORD=synthetic-secret-must-not-escape"],"Labels":{"com.docker.compose.service":"postgresql","unrelated":"private"}},"State":{"Running":true,"Health":{"Status":"healthy"}},"HostConfig":{"NetworkMode":"bridge"},"Mounts":[{"Name":"bitnami-postgresql-data","Destination":"/bitnami/postgresql"}],"NetworkSettings":{"Networks":{"bridge":{"Aliases":["postgres"]}},"Ports":{"5432/tcp":[{"HostIp":"127.0.0.1","HostPort":"5432"}]}}}]`
	runner := &fakeDockerRunner{outputs: [][]byte{[]byte(id + "\tbitnami/postgresql:11-debian-10\n"), []byte(inspect)}}
	inspector := DockerCLIInspector{Runner: runner}
	candidates, err := inspector.Candidates(context.Background(), PostgreSQL11Image)
	if err != nil || len(candidates) != 1 || candidates[0].ID != id {
		t.Fatalf("Candidates() = %#v, %v", candidates, err)
	}
	details, err := inspector.Inspect(context.Background(), id)
	if err != nil || details.Health != HealthHealthy || len(details.DatabaseNames) != 1 || details.DatabaseNames[0] != "alice" || details.Labels.ComposeService != "postgresql" {
		t.Fatalf("Inspect() = %#v, %v", details, err)
	}
	assertNoDockerLeak(t, details, candidates, err)
	if len(runner.calls) != 2 || runner.calls[0] != "docker ps --all --no-trunc --format {{.ID}}\t{{.Image}}" || runner.calls[1] != "docker inspect "+id {
		t.Fatalf("calls = %v", runner.calls)
	}
}

type fakeInspector struct {
	candidates []ContainerSummary
	details    map[string]ContainerDetails
	inspectErr map[string]error
	inspected  []string
}

func (f *fakeInspector) Candidates(context.Context, ImageIdentity) ([]ContainerSummary, error) {
	return f.candidates, nil
}
func (f *fakeInspector) Inspect(_ context.Context, id string) (ContainerDetails, error) {
	f.inspected = append(f.inspected, id)
	if err := f.inspectErr[id]; err != nil {
		return ContainerDetails{}, err
	}
	return f.details[id], nil
}

type fakeDockerRunner struct {
	outputs [][]byte
	errs    []error
	calls   []string
}

func (f *fakeDockerRunner) Run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		return nil, nil, err
	}
	out := f.outputs[0]
	f.outputs = f.outputs[1:]
	return out, nil, nil
}
func assertNoDockerLeak(t *testing.T, values ...any) {
	t.Helper()
	rendered := fmt.Sprint(values...)
	for _, forbidden := range []string{"synthetic-secret-must-not-escape", "unrelated", "private", "POSTGRESQL_PASSWORD", "secret-env"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("docker metadata leaked %q in %q", forbidden, rendered)
		}
	}
}
