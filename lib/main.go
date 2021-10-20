package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	BuildkiteIdSuffix  = "/Attestations/Buildkite@v1"
	TypeId             = "https://buildkite.com/Attestations/BuildkiteBuild@v1"
	PayloadContentType = "application/vnd.in-toto+json"
)

type arrayFlags []string

func (i *arrayFlags) String() string {
	return ""
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

var (
	artifactPath arrayFlags
	outputPath   = flag.String("output_path", "provenance.json", "The path to which the generated provenance should be written.")
	buildContext = flag.String("build_context", "", "The '${build}' context value.")
	agentContext = flag.String("agent_context", "", "The '${agent}' context value.")
)

var (
	// scpSyntax was modified from https://golang.org/src/cmd/go/vcs.go.
	scpSyntax = regexp.MustCompile(`^([a-zA-Z0-9-._~]+@)?([a-zA-Z0-9._-]+):([a-zA-Z0-9./._-]+)(?:\?||$)(.*)$`)

	// Transports is a set of known Git URL schemes.
	Transports = NewTransportSet(
		"ssh",
		"git",
		"git+ssh",
		"http",
		"https",
		"ftp",
		"ftps",
		"rsync",
		"file",
	)
)

// Parser converts a string into a URL.
type Parser func(string) (*url.URL, error)

// Parse parses rawurl into a URL structure. Parse first attempts to
// find a standard URL with a valid Git transport as its scheme. If
// that cannot be found, it then attempts to find a SCP-like URL. And
// if that cannot be found, it assumes rawurl is a local path. If none
// of these rules apply, Parse returns an error.
func Parse(rawurl string) (u *url.URL, err error) {
	parsers := []Parser{
		ParseTransport,
		ParseScp,
		ParseLocal,
	}

	// Apply each parser in turn; if the parser succeeds, accept its
	// result and return.
	for _, p := range parsers {
		u, err = p(rawurl)
		if err == nil {
			return u, err
		}
	}

	// It's unlikely that none of the parsers will succeed, since
	// ParseLocal is very forgiving.
	return new(url.URL), fmt.Errorf("failed to parse %q", rawurl)
}

// ParseTransport parses rawurl into a URL object. Unless the URL's
// scheme is a known Git transport, ParseTransport returns an error.
func ParseTransport(rawurl string) (*url.URL, error) {
	u, err := url.Parse(rawurl)
	if err == nil && !Transports.Valid(u.Scheme) {
		err = fmt.Errorf("scheme %q is not a valid transport", u.Scheme)
	}
	return u, err
}

// ParseScp parses rawurl into a URL object. The rawurl must be
// an SCP-like URL, otherwise ParseScp returns an error.
func ParseScp(rawurl string) (*url.URL, error) {
	match := scpSyntax.FindAllStringSubmatch(rawurl, -1)
	if len(match) == 0 {
		return nil, fmt.Errorf("no scp URL found in %q", rawurl)
	}
	m := match[0]
	user := strings.TrimRight(m[1], "@")
	var userinfo *url.Userinfo
	if user != "" {
		userinfo = url.User(user)
	}
	rawquery := ""
	if len(m) > 3 {
		rawquery = m[4]
	}
	return &url.URL{
		Scheme:   "ssh",
		User:     userinfo,
		Host:     m[2],
		Path:     m[3],
		RawQuery: rawquery,
	}, nil
}

// ParseLocal parses rawurl into a URL object with a "file"
// scheme. This will effectively never return an error.
func ParseLocal(rawurl string) (*url.URL, error) {
	return &url.URL{
		Scheme: "file",
		Host:   "",
		Path:   rawurl,
	}, nil
}

// TransportSet represents a set of valid Git transport schemes. It
// maps these schemes to empty structs, providing a set-like
// interface.
type TransportSet struct {
	Transports map[string]struct{}
}

// NewTransportSet returns a TransportSet with the items keys mapped
// to empty struct values.
func NewTransportSet(items ...string) *TransportSet {
	t := &TransportSet{
		Transports: map[string]struct{}{},
	}
	for _, i := range items {
		t.Transports[i] = struct{}{}
	}
	return t
}

// Valid returns true if transport is a known Git URL scheme and false
// if not.
func (t *TransportSet) Valid(transport string) bool {
	_, ok := t.Transports[transport]
	return ok
}

type Envelope struct {
	PayloadType string        `json:"payloadType"`
	Payload     string        `json:"payload"`
	Signatures  []interface{} `json:"signatures"`
}
type Statement struct {
	Type          string    `json:"_type"`
	Subject       []Subject `json:"subject"`
	PredicateType string    `json:"predicateType"`
	Predicate     `json:"predicate"`
}
type Subject struct {
	Name   string    `json:"name"`
	Digest DigestSet `json:"digest"`
}
type Predicate struct {
	Builder   `json:"builder"`
	Metadata  `json:"metadata"`
	Recipe    `json:"recipe"`
	Materials []Item `json:"materials"`
}
type Builder struct {
	Id string `json:"id"`
}
type Metadata struct {
	BuildInvocationId string `json:"buildInvocationId"`
	Completeness      `json:"completeness"`
	Reproducible      bool `json:"reproducible"`
	// BuildStartedOn not defined as it's not available from a GitHub Action.
	BuildFinishedOn string `json:"buildFinishedOn"`
}
type Recipe struct {
	Type              string          `json:"type"`
	DefinedInMaterial int             `json:"definedInMaterial"`
	EntryPoint        string          `json:"entryPoint"`
	Arguments         json.RawMessage `json:"arguments"`
	Environment       *AnyContext     `json:"environment"`
}
type Completeness struct {
	Arguments   bool `json:"arguments"`
	Environment bool `json:"environment"`
	Materials   bool `json:"materials"`
}
type DigestSet map[string]string
type Item struct {
	URI    string    `json:"uri"`
	Digest DigestSet `json:"digest"`
}

type AnyContext struct {
	BuildContext `json:"build"`
	AgentContext `json:"agent"`
}

type BuildContext struct {
	Repository string `json:"repository"`
	BuildURL   string `json:"build_url"`
	Commit     string `json:"commit"`
	StepID     string `json:"step_id"`
	Command    string `json:"command"`
}

type AgentContext struct {
	Name         string `json:"agent_name"`
	ID           string `json:"agent_id"`
	Organization string `json:"agent_organization"`
}

// subjects walks the file or directory at "root" and hashes all files.
func subjects(root string) ([]Subject, error) {
	var s []Subject
	return s, filepath.Walk(root, func(abspath string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relpath, err := filepath.Rel(root, abspath)
		if err != nil {
			return err
		}
		// Note: filepath.Rel() returns "." when "root" and "abspath" point to the same file.
		if relpath == "." {
			relpath = filepath.Base(root)
		}
		contents, err := ioutil.ReadFile(abspath)
		if err != nil {
			return err
		}
		sha := sha256.Sum256(contents)
		shaHex := hex.EncodeToString(sha[:])
		s = append(s, Subject{Name: relpath, Digest: DigestSet{"sha256": shaHex}})
		return nil
	})
}

func parseFlags() {
	flag.Parse()
	if len(artifactPath) < 1 {
		fmt.Println("No value found for required flag: --artifact_path\n")
		flag.Usage()
		os.Exit(1)
	}
	if *outputPath == "" {
		fmt.Println("No value found for required flag: --output_path\n")
		flag.Usage()
		os.Exit(1)
	}
	if *buildContext == "" {
		fmt.Println("No value found for required flag: --build_context\n")
		flag.Usage()
		os.Exit(1)
	}
	if *agentContext == "" {
		fmt.Println("No value found for required flag: --agent_context\n")
		flag.Usage()
		os.Exit(1)
	}
}

func EscapedMarshal(t interface{}) ([]byte, error) {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(t)
	return buffer.Bytes(), err
}

func EscapedMarshalIndent(v interface{}, prefix, indent string) ([]byte, error) {
	b, err := EscapedMarshal(v)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	err = json.Indent(&buf, b, prefix, indent)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func main() {
	flag.Var(&artifactPath, "artifact_path", "The file or dir path of the artifacts for which provenance should be generated.")
	parseFlags()
	stmt := Statement{PredicateType: "https://slsa.dev/provenance/v0.1", Type: "https://in-toto.io/Statement/v0.1"}

	var allSubjects []Subject
	for _, path := range artifactPath {
		subjects, err := subjects(path)
		if os.IsNotExist(err) {
			fmt.Println(fmt.Sprintf("Resource path not found: [provided=%s]", path))
			os.Exit(1)
		} else if err != nil {
			panic(err)
		}
		allSubjects = append(allSubjects, subjects...)
	}
	stmt.Subject = append(stmt.Subject, allSubjects...)
	stmt.Predicate = Predicate{
		Builder{},
		Metadata{
			Completeness: Completeness{
				Arguments:   true,
				Environment: false,
				Materials:   false,
			},
			Reproducible:    false,
			BuildFinishedOn: time.Now().UTC().Format(time.RFC3339),
		},
		Recipe{
			Type:              TypeId,
			DefinedInMaterial: 0,
		},
		[]Item{},
	}

	context := AnyContext{}
	if err := json.Unmarshal([]byte(*buildContext), &context.BuildContext); err != nil {
		panic(err)
	}
	if err := json.Unmarshal([]byte(*agentContext), &context.AgentContext); err != nil {
		panic(err)
	}
	build := context.BuildContext
	agent := context.AgentContext

	repositoryURL, err := Parse(build.Repository)

	if err != nil {
		panic(err)
	}

	materialsURI := "git+https://" + repositoryURL.Host + "/" + strings.Replace(repositoryURL.Path, ".git", "", 1)

	stmt.Predicate.Metadata.BuildInvocationId = build.BuildURL
	stmt.Predicate.Recipe.EntryPoint = build.Command
	stmt.Predicate.Materials = append(stmt.Predicate.Materials, Item{URI: materialsURI, Digest: DigestSet{"sha1": build.Commit}})
	stmt.Predicate.Builder.Id = "https://buildkite.com/organizations/" + agent.Organization + "/agents/" + agent.ID

	// NOTE: At L1, writing the in-toto Statement type is sufficient but, at
	// higher SLSA levels, the Statement must be encoded and wrapped in an
	// Envelope to support attaching signatures.
	payload, _ := EscapedMarshalIndent(stmt, "", "  ")
	fmt.Println("Provenance:\n" + string(payload))
	if err := ioutil.WriteFile(*outputPath, payload, 0755); err != nil {
		fmt.Println("Failed to write provenance: %s", err)
		os.Exit(1)
	}
}
