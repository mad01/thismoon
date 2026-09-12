package mcpformat

import "testing"

type fixtureRepo struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Dirty bool   `json:"dirty"`
}

type fixtureSemantic struct {
	Enabled bool   `json:"enabled"`
	Model   string `json:"model"`
}

// fixture covers every shape the encoders handle: scalars, an omitempty field
// left empty, a record array, a nested object, a primitive array, a
// multi-line string, an empty array, and a nil pointer. The record array sits
// in the middle so meta fields on both sides of it are exercised.
type fixture struct {
	Tool      string          `json:"tool"`
	Total     int             `json:"total"`
	Truncated bool            `json:"truncated"`
	Note      string          `json:"note,omitempty"`
	Repos     []fixtureRepo   `json:"repos"`
	Semantic  fixtureSemantic `json:"semantic"`
	Tags      []string        `json:"tags"`
	Hint      string          `json:"hint"`
	Dropped   []fixtureRepo   `json:"dropped"`
	Warning   *string         `json:"warning"`
}

func newFixture() fixture {
	return fixture{
		Tool:  "csl_repo_info",
		Total: 3,
		Repos: []fixtureRepo{
			{Name: "mad01/thismoon", Path: "/code/thismoon", Dirty: true},
			{Name: "mad01/ralph", Path: "/code/ralph"},
			{Name: "mad01/kitty-session", Path: "/code/kitty-session"},
		},
		Semantic: fixtureSemantic{Enabled: true, Model: "jina-code-v2"},
		Tags:     []string{"go", "mcp", "local"},
		Hint:     "first line\nsecond line",
		Dropped:  []fixtureRepo{},
	}
}

// flatFixture has no array of objects, so it takes every encoder's
// no-primary-array branch.
type flatFixture struct {
	OK       bool            `json:"ok"`
	Checks   int             `json:"checks"`
	Semantic fixtureSemantic `json:"semantic"`
}

func newFlatFixture() flatFixture {
	return flatFixture{OK: true, Checks: 2, Semantic: fixtureSemantic{Model: "none"}}
}

// raggedFixture has records with differing keys; map keys marshal sorted, so
// the expected header is the first-seen union a, b, c.
type raggedFixture struct {
	Items []map[string]any `json:"items"`
}

func newRaggedFixture() raggedFixture {
	return raggedFixture{Items: []map[string]any{{"a": 1, "b": 2}, {"a": 3, "c": 4}}}
}

// nestedRecordFixture puts an object and a primitive array inside a record.
type nestedRecordFixture struct {
	Hits []nestedHit `json:"hits"`
}

type nestedHit struct {
	Repo  string   `json:"repo"`
	Span  span     `json:"span"`
	Langs []string `json:"langs"`
}

type span struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

func newNestedRecordFixture() nestedRecordFixture {
	return nestedRecordFixture{Hits: []nestedHit{{Repo: "r", Span: span{Start: 1, End: 2}, Langs: []string{"go"}}}}
}

type encodeCase struct {
	name string
	in   any
	want string
}

func runEncodeCases(t *testing.T, format string, cases []encodeCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Encode(format, tc.in)
			if err != nil {
				t.Fatalf("Encode(%q) error = %v", format, err)
			}
			if got != tc.want {
				t.Fatalf("Encode(%q) mismatch\n got: %q\nwant: %q\n--- got ---\n%s\n--- want ---\n%s", format, got, tc.want, got, tc.want)
			}
		})
	}
}

func TestEncodeJSON(t *testing.T) {
	runEncodeCases(t, JSON, []encodeCase{
		{
			name: "fixture",
			in:   newFixture(),
			want: `{"tool":"csl_repo_info","total":3,"truncated":false,"repos":[{"name":"mad01/thismoon","path":"/code/thismoon","dirty":true},{"name":"mad01/ralph","path":"/code/ralph","dirty":false},{"name":"mad01/kitty-session","path":"/code/kitty-session","dirty":false}],"semantic":{"enabled":true,"model":"jina-code-v2"},"tags":["go","mcp","local"],"hint":"first line\nsecond line","dropped":[],"warning":null}`,
		},
	})
}

func TestEncodeJSONL(t *testing.T) {
	runEncodeCases(t, JSONL, []encodeCase{
		{
			name: "fixture",
			in:   newFixture(),
			want: `{"tool":"csl_repo_info","total":3,"truncated":false,"semantic":{"enabled":true,"model":"jina-code-v2"},"tags":["go","mcp","local"],"hint":"first line\nsecond line","dropped":[],"warning":null}
{"name":"mad01/thismoon","path":"/code/thismoon","dirty":true}
{"name":"mad01/ralph","path":"/code/ralph","dirty":false}
{"name":"mad01/kitty-session","path":"/code/kitty-session","dirty":false}
`,
		},
		{
			name: "no primary array is one line",
			in:   newFlatFixture(),
			want: `{"ok":true,"checks":2,"semantic":{"enabled":false,"model":"none"}}` + "\n",
		},
	})
}

func TestEncodeCSV(t *testing.T) {
	runEncodeCases(t, CSV, []encodeCase{
		{
			name: "fixture",
			in:   newFixture(),
			want: `# tool: csl_repo_info
# total: 3
# truncated: false
# semantic: {"enabled":true,"model":"jina-code-v2"}
# tags: ["go","mcp","local"]
# hint: "first line\nsecond line"
# dropped: []
# warning:
name,path,dirty
mad01/thismoon,/code/thismoon,true
mad01/ralph,/code/ralph,false
mad01/kitty-session,/code/kitty-session,false
`,
		},
		{
			name: "ragged records union keys and leave absent cells empty",
			in:   newRaggedFixture(),
			want: "a,b,c\n1,2,\n3,,4\n",
		},
		{
			name: "nested record values are compact JSON cells",
			in:   newNestedRecordFixture(),
			want: "repo,span,langs\n" + `r,"{""start"":1,""end"":2}","[""go""]"` + "\n",
		},
		{
			name: "no primary array is a key,value table",
			in:   newFlatFixture(),
			want: "key,value\nok,true\nchecks,2\n" + `semantic,"{""enabled"":false,""model"":""none""}"` + "\n",
		},
	})
}

func TestEncodeMarkdownKV(t *testing.T) {
	runEncodeCases(t, MarkdownKV, []encodeCase{
		{
			name: "fixture",
			in:   newFixture(),
			want: `tool: csl_repo_info
total: 3
truncated: false
semantic.enabled: true
semantic.model: jina-code-v2
tags: go, mcp, local
hint: |
  first line
  second line
dropped: (none)
warning: null

## repos (3)
name: mad01/thismoon
path: /code/thismoon
dirty: true

name: mad01/ralph
path: /code/ralph
dirty: false

name: mad01/kitty-session
path: /code/kitty-session
dirty: false
`,
		},
		{
			name: "records flatten nested objects and join primitive arrays",
			in:   newNestedRecordFixture(),
			want: "## hits (1)\nrepo: r\nspan.start: 1\nspan.end: 2\nlangs: go\n",
		},
		{
			name: "no primary array is the scalar block alone",
			in:   newFlatFixture(),
			want: "ok: true\nchecks: 2\nsemantic.enabled: false\nsemantic.model: none\n",
		},
	})
}

func TestEncodeXML(t *testing.T) {
	runEncodeCases(t, XML, []encodeCase{
		{
			name: "fixture",
			in:   newFixture(),
			want: `<result>
<tool>csl_repo_info</tool>
<total>3</total>
<truncated>false</truncated>
<repos>
<item>
<name>mad01/thismoon</name>
<path>/code/thismoon</path>
<dirty>true</dirty>
</item>
<item>
<name>mad01/ralph</name>
<path>/code/ralph</path>
<dirty>false</dirty>
</item>
<item>
<name>mad01/kitty-session</name>
<path>/code/kitty-session</path>
<dirty>false</dirty>
</item>
</repos>
<semantic>
<enabled>true</enabled>
<model>jina-code-v2</model>
</semantic>
<tags>
<item>go</item>
<item>mcp</item>
<item>local</item>
</tags>
<hint>first line&#xA;second line</hint>
<dropped></dropped>
<warning></warning>
</result>
`,
		},
		{
			name: "text is escaped",
			in: struct {
				Q string `json:"q"`
			}{Q: `a<b & "c"`},
			want: "<result>\n<q>a&lt;b &amp; &#34;c&#34;</q>\n</result>\n",
		},
	})
}

func TestEncodeTOON(t *testing.T) {
	runEncodeCases(t, TOON, []encodeCase{
		{
			name: "fixture sorts keys and tabulates records",
			in:   newFixture(),
			want: `dropped[0]:
hint: "first line\nsecond line"
repos[3]{dirty,name,path}:
  true,mad01/thismoon,/code/thismoon
  false,mad01/ralph,/code/ralph
  false,mad01/kitty-session,/code/kitty-session
semantic:
  enabled: true
  model: jina-code-v2
tags[3]: go,mcp,local
tool: csl_repo_info
total: 3
truncated: false
warning: null
`,
		},
		{
			name: "no primary array",
			in:   newFlatFixture(),
			want: "checks: 2\nok: true\nsemantic:\n  enabled: false\n  model: none\n",
		},
	})
}
