package workspace

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"testing"

	"github.com/databrickslabs/databricks-terraform/common"
	"github.com/databrickslabs/databricks-terraform/internal/qa"

	"github.com/stretchr/testify/assert"
)

func notebookToB64(filePath string) (string, error) {
	notebookBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("unable to find notebook to convert to base64; %w", err)
	}
	return base64.StdEncoding.EncodeToString(notebookBytes), nil
}

func TestValidateNotebookPath(t *testing.T) {
	testCases := []struct {
		name         string
		notebookPath string
		errorCount   int
	}{
		{"empty_path",
			"",
			2},
		{"correct_path",
			"/directory",
			0},
		{"path_starts_with_no_slash",
			"directory",
			1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := ValidateNotebookPath(tc.notebookPath, "key")

			assert.Lenf(t, errs, tc.errorCount, "directory '%s' does not generate the expected error count", tc.notebookPath)
		})
	}
}

func TestResourceNotebookCreate_DirDoesNotExists(t *testing.T) {
	pythonNotebookDataB64, err := notebookToB64("testdata/tf-test-python.py")
	assert.NoError(t, err, err)
	checkSum, err := convertBase64ToCheckSum(pythonNotebookDataB64)
	assert.NoError(t, err, err)
	path := "/test/path.py"
	content := pythonNotebookDataB64
	objectId := 12345

	d, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/workspace/get-status?path=%2Ftest",
				Response: common.APIErrorBody{
					ErrorCode: "NOT_FOUND",
					Message:   "not found",
				},
				Status: 404,
			},
			{
				Method:   http.MethodPost,
				Resource: "/api/2.0/workspace/mkdirs",
				Response: NotebookImportRequest{
					Content:   content,
					Path:      path,
					Language:  Python,
					Overwrite: true,
					Format:    Source,
				},
			},
			{
				Method:   http.MethodPost,
				Resource: "/api/2.0/workspace/import",
				Response: NotebookImportRequest{
					Content:   content,
					Path:      path,
					Language:  Python,
					Overwrite: true,
					Format:    Source,
				},
			},
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/workspace/export?format=SOURCE&path=%2Ftest%2Fpath.py",
				Response: NotebookContent{
					Content: pythonNotebookDataB64,
				},
			},
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/workspace/get-status?path=%2Ftest%2Fpath.py",
				Response: WorkspaceObjectStatus{
					ObjectID:   int64(objectId),
					ObjectType: Notebook,
					Path:       path,
					Language:   Python,
				},
			},
		},
		Resource: ResourceNotebook(),
		State: map[string]interface{}{
			"path":      path,
			"content":   content,
			"language":  string(Python),
			"format":    string(Source),
			"overwrite": true,
			"mkdirs":    true,
		},
		Create: true,
	}.Apply(t)
	assert.NoError(t, err, err)
	assert.Equal(t, path, d.Id())
	assert.Equal(t, checkSum, d.Get("content"))
	assert.Equal(t, path, d.Get("path"))
	assert.Equal(t, string(Python), d.Get("language"))
	assert.Equal(t, objectId, d.Get("object_id"))
}

func TestResourceNotebookCreate_NoMkdirs(t *testing.T) {
	pythonNotebookDataB64, err := notebookToB64("testdata/tf-test-python.py")
	assert.NoError(t, err, err)
	checkSum, err := convertBase64ToCheckSum(pythonNotebookDataB64)
	assert.NoError(t, err, err)
	path := "/test/path.py"
	content := pythonNotebookDataB64
	objectId := 12345

	d, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			{
				Method:   http.MethodPost,
				Resource: "/api/2.0/workspace/import",
				Response: NotebookImportRequest{
					Content:   content,
					Path:      path,
					Language:  Python,
					Overwrite: true,
					Format:    Source,
				},
			},
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/workspace/export?format=SOURCE&path=%2Ftest%2Fpath.py",
				Response: NotebookContent{
					Content: pythonNotebookDataB64,
				},
			},
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/workspace/get-status?path=%2Ftest%2Fpath.py",
				Response: WorkspaceObjectStatus{
					ObjectID:   int64(objectId),
					ObjectType: Notebook,
					Path:       path,
					Language:   Python,
				},
			},
		},
		Resource: ResourceNotebook(),
		State: map[string]interface{}{
			"path":      path,
			"content":   content,
			"language":  string(Python),
			"format":    string(Source),
			"overwrite": true,
			"mkdirs":    false,
		},
		Create: true,
	}.Apply(t)
	assert.NoError(t, err, err)
	assert.Equal(t, path, d.Id())
	assert.Equal(t, checkSum, d.Get("content"))
	assert.Equal(t, path, d.Get("path"))
	assert.Equal(t, string(Python), d.Get("language"))
	assert.Equal(t, objectId, d.Get("object_id"))
}

func TestResourceNotebookRead(t *testing.T) {
	pythonNotebookDataB64, err := notebookToB64("testdata/tf-test-python.py")
	assert.NoError(t, err, err)
	checkSum, err := convertBase64ToCheckSum(pythonNotebookDataB64)
	assert.NoError(t, err, err)
	exportFormat := Source
	testId := "/test/path.py"
	objectId := 12345
	assert.NoError(t, err, err)
	d, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/workspace/export?format=SOURCE&path=%2Ftest%2Fpath.py",
				Response: NotebookContent{
					Content: pythonNotebookDataB64,
				},
			},
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/workspace/get-status?path=%2Ftest%2Fpath.py",
				Response: WorkspaceObjectStatus{
					ObjectID:   int64(objectId),
					ObjectType: Notebook,
					Path:       testId,
					Language:   Python,
				},
			},
		},
		Resource: ResourceNotebook(),
		Read:     true,
		ID:       testId,
		State: map[string]interface{}{
			"format": exportFormat,
		},
	}.Apply(t)
	assert.NoError(t, err, err)
	assert.Equal(t, testId, d.Id())
	assert.Equal(t, checkSum, d.Get("content"))
	assert.Equal(t, testId, d.Get("path"))
	assert.Equal(t, string(Python), d.Get("language"))
	assert.Equal(t, objectId, d.Get("object_id"))
}

func TestResourceNotebookDelete(t *testing.T) {
	testId := "/test/path.py"
	d, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			{
				Method:          http.MethodPost,
				Resource:        "/api/2.0/workspace/delete",
				Status:          http.StatusOK,
				ExpectedRequest: NotebookDeleteRequest{Path: testId, Recursive: true},
			},
		},
		Resource: ResourceNotebook(),
		Delete:     true,
		ID:       testId,
	}.Apply(t)
	assert.NoError(t, err, err)
	assert.Equal(t, testId, d.Id())
}

func TestResourceNotebookDelete_TooManyRequests(t *testing.T) {
	testId := "/test/path.py"
	d, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			{
				Method:   http.MethodPost,
				Resource: "/api/2.0/workspace/delete",
				Status:   http.StatusTooManyRequests,
			},
			{
				Method:          http.MethodPost,
				Resource:        "/api/2.0/workspace/delete",
				Status:          http.StatusOK,
				ExpectedRequest: NotebookDeleteRequest{Path: testId, Recursive: true},
			},
		},
		Resource: ResourceNotebook(),
		Delete:   true,
		ID:       testId,
	}.Apply(t)
	assert.NoError(t, err, err)
	assert.Equal(t, testId, d.Id())
}

func TestResourceNotebookRead_NotFound(t *testing.T) {
	d, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			{ // read log output for correct url...
				Method:   "GET",
				Resource: "/api/2.0/workspace/export?format=SOURCE&path=%2Ftest%2Fpath.py",
				Response: common.APIErrorBody{
					ErrorCode: "NOT_FOUND",
					Message:   "Item not found",
				},
				Status: 404,
			},
		},
		Resource: ResourceNotebook(),
		Read:     true,
		ID:       "/test/path.py",
	}.Apply(t)
	assert.NoError(t, err, err)
	assert.Equal(t, "", d.Id(), "Id should be empty for missing resources")
}

func TestResourceNotebookRead_Error(t *testing.T) {
	d, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			{ // read log output for correct url...
				Method:   "GET",
				Resource: "/api/2.0/workspace/export?format=SOURCE&path=%2Ftest%2Fpath.py",
				Response: common.APIErrorBody{
					ErrorCode: "INVALID_REQUEST",
					Message:   "Internal error happened",
				},
				Status: 400,
			},
		},
		Resource: ResourceNotebook(),
		Read:     true,
		ID:       "/test/path.py",
	}.Apply(t)
	qa.AssertErrorStartsWith(t, err, "Internal error happened")
	assert.Equal(t, "/test/path.py", d.Id(), "Id should not be empty for error reads")
}

func TestResourceNotebookCreate(t *testing.T) {
	d, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			{
				Method:   http.MethodPost,
				Resource: "/api/2.0/workspace/import",
				Response: NotebookImportRequest{
					Content:   "YWJjCg==",
					Path:      "/path.py",
					Language:  Python,
					Overwrite: true,
					Format:    Source,
				},
			},
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/workspace/export?format=SOURCE&path=%2Fpath.py",
				Response: NotebookContent{
					Content: "YWJjCg==",
				},
			},
			{
				Method:   http.MethodGet,
				Resource: "/api/2.0/workspace/get-status?path=%2Fpath.py",
				Response: WorkspaceObjectStatus{
					ObjectID:   4567,
					ObjectType: "NOTEBOOK",
					Path:       "/path.py",
					Language:   Python,
				},
			},
		},
		Resource: ResourceNotebook(),
		State: map[string]interface{}{
			"content":   "YWJjCg==",
			"format":    "SOURCE",
			"language":  "PYTHON",
			"overwrite": true,
			"path":      "/path.py",
		},
		Create: true,
	}.Apply(t)
	assert.NoError(t, err, err)
	assert.Equal(t, "/path.py", d.Id())
}

func TestResourceNotebookCreate_Error(t *testing.T) {
	d, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			{
				Method:   http.MethodPost,
				Resource: "/api/2.0/workspace/import",
				Response: common.APIErrorBody{
					ErrorCode: "INVALID_REQUEST",
					Message:   "Internal error happened",
				},
				Status: 400,
			},
		},
		Resource: ResourceNotebook(),
		State: map[string]interface{}{
			"content":   "YWJjCg==",
			"format":    "SOURCE",
			"language":  "PYTHON",
			"overwrite": true,
			"path":      "/path.py",
		},
		Create: true,
	}.Apply(t)
	qa.AssertErrorStartsWith(t, err, "Internal error happened")
	assert.Equal(t, "", d.Id(), "Id should be empty for error creates")
}

func TestResourceNotebookDelete_Error(t *testing.T) {
	d, err := qa.ResourceFixture{
		Fixtures: []qa.HTTPFixture{
			{
				Method:   "POST",
				Resource: "/api/2.0/workspace/delete",
				Response: common.APIErrorBody{
					ErrorCode: "INVALID_REQUEST",
					Message:   "Internal error happened",
				},
				Status: 400,
			},
		},
		Resource: ResourceNotebook(),
		Delete:   true,
		ID:       "abc",
	}.Apply(t)
	qa.AssertErrorStartsWith(t, err, "Internal error happened")
	assert.Equal(t, "abc", d.Id())
}

func TestNotebooksAPI_Create(t *testing.T) {
	type args struct {
		Content   string       `json:"content,omitempty"`
		Path      string       `json:"path,omitempty"`
		Language  Language     `json:"language,omitempty"`
		Overwrite bool         `json:"overwrite,omitempty"`
		Format    ExportFormat `json:"format,omitempty"`
	}

	tests := []struct {
		name     string
		response string
		args     args
		wantErr  bool
	}{
		{
			name:     "Create Test",
			response: "",
			args: args{
				Content:   "helloworld",
				Path:      "my-path",
				Language:  Python,
				Overwrite: false,
				Format:    DBC,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input args
			qa.AssertRequestWithMockServer(t, &tt.args, http.MethodPost, "/api/2.0/workspace/import", &input, tt.response, http.StatusOK, nil,
				tt.wantErr, func(client common.DatabricksClient) (interface{}, error) {
					return nil, NewNotebooksAPI(&client).Create(tt.args.Path, tt.args.Content, tt.args.Language, tt.args.Format, tt.args.Overwrite)
				})
		})
	}
}

func TestNotebooksAPI_MkDirs(t *testing.T) {
	type args struct {
		Path string `json:"path,omitempty"`
	}

	tests := []struct {
		name     string
		response string
		args     args
		wantErr  bool
	}{
		{
			name:     "Create Test",
			response: "",
			args: args{
				Path: "/test/path",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input args
			qa.AssertRequestWithMockServer(t, &tt.args, http.MethodPost, "/api/2.0/workspace/mkdirs", &input, tt.response, http.StatusOK, nil, tt.wantErr, func(client common.DatabricksClient) (interface{}, error) {
				return nil, NewNotebooksAPI(&client).Mkdirs(tt.args.Path)
			})
		})
	}
}

func TestNotebooksAPI_Delete(t *testing.T) {
	type args struct {
		Path      string `json:"path,omitempty"`
		Recursive bool   `json:"recursive,omitempty"`
	}
	tests := []struct {
		name           string
		response       string
		responseStatus int
		args           args
		wantErr        bool
	}{
		{
			name:           "Delete test",
			response:       "",
			responseStatus: http.StatusOK,
			args: args{
				Path:      "mypath",
				Recursive: false,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input args
			qa.AssertRequestWithMockServer(t, &tt.args, http.MethodPost, "/api/2.0/workspace/delete", &input, tt.response, tt.responseStatus, nil, tt.wantErr, func(client common.DatabricksClient) (interface{}, error) {
				return nil, NewNotebooksAPI(&client).Delete(tt.args.Path, tt.args.Recursive)
			})
		})
	}
}

func TestNotebooksAPI_ListNonRecursive(t *testing.T) {
	type args struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
	}
	tests := []struct {
		name           string
		response       string
		responseStatus int
		args           args
		wantURI        string
		want           []WorkspaceObjectStatus
		wantErr        bool
	}{
		{
			name: "List non recursive test",
			response: `{
						  "objects": [
							{
							  "path": "/Users/user@example.com/project",
							  "object_type": "DIRECTORY",
							  "object_id": 123
							},
							{
							  "path": "/Users/user@example.com/PythonExampleNotebook",
							  "language": "PYTHON",
							  "object_type": "NOTEBOOK",
							  "object_id": 456
							}
						  ]
						}`,
			responseStatus: http.StatusOK,
			args: args{

				Path:      "/test/path",
				Recursive: false,
			},
			wantURI: "/api/2.0/workspace/list?path=%2Ftest%2Fpath",
			want: []WorkspaceObjectStatus{
				{
					ObjectID:   123,
					ObjectType: Directory,
					Path:       "/Users/user@example.com/project",
				},
				{
					ObjectID:   456,
					ObjectType: Notebook,
					Language:   Python,
					Path:       "/Users/user@example.com/PythonExampleNotebook",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input args
			qa.AssertRequestWithMockServer(t, tt.args, http.MethodGet, tt.wantURI, &input, tt.response, tt.responseStatus, tt.want, tt.wantErr, func(client common.DatabricksClient) (interface{}, error) {
				return NewNotebooksAPI(&client).List(tt.args.Path, tt.args.Recursive)
			})
		})
	}
}

func TestNotebooksAPI_ListRecursive(t *testing.T) {
	type args struct {
		Path      string `json:"path"`
		Recursive bool   `json:"recursive"`
	}
	tests := []struct {
		name           string
		response       []string
		responseStatus []int
		args           []interface{}
		wantURI        []string
		want           []WorkspaceObjectStatus
		wantErr        bool
	}{
		{
			name: "List recursive test",
			response: []string{`{
						  "objects": [
							{
							  "path": "/Users/user@example.com/project",
							  "object_type": "DIRECTORY",
							  "object_id": 123
							},
							{
							  "path": "/Users/user@example.com/PythonExampleNotebook",
							  "language": "PYTHON",
							  "object_type": "NOTEBOOK",
							  "object_id": 456
							}
						  ]
						}`,
				`{
						  "objects": [
							{
							  "path": "/Users/user@example.com/Notebook2",
							  "language": "PYTHON",
							  "object_type": "NOTEBOOK",
							  "object_id": 457
							}
						  ]
						}`,
			},
			responseStatus: []int{http.StatusOK, http.StatusOK},
			args: []interface{}{
				&args{
					Path:      "/test/path",
					Recursive: true,
				},
			},
			wantURI: []string{"/api/2.0/workspace/list?path=%2Ftest%2Fpath", "/api/2.0/workspace/list?path=%2FUsers%2Fuser@example.com%2Fproject"},
			want: []WorkspaceObjectStatus{
				{
					ObjectID:   457,
					ObjectType: Notebook,
					Language:   Python,
					Path:       "/Users/user@example.com/Notebook2",
				},
				{
					ObjectID:   456,
					ObjectType: Notebook,
					Language:   Python,
					Path:       "/Users/user@example.com/PythonExampleNotebook",
				},
			},
			wantErr: false,
		},
		{
			name: "List recursive test failure",
			response: []string{`{
						  "objects": [
							{
							  "path": "/Users/user@example.com/project",
							  "object_type": "DIRECTORY",
							  "object_id": 123
							},
							{
							  "path": "/Users/user@example.com/PythonExampleNotebook",
							  "language": "PYTHON",
							  "object_type": "NOTEBOOK",
							  "object_id": 456
							}
						  ]
						}`,
				``,
			},
			responseStatus: []int{http.StatusOK, http.StatusBadRequest},
			args: []interface{}{
				&args{
					Path:      "/test/path",
					Recursive: true,
				},
			},
			wantURI: []string{"/api/2.0/workspace/list?path=%2Ftest%2Fpath", "/api/2.0/workspace/list?path=%2FUsers%2Fuser@example.com%2Fproject"},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qa.AssertMultipleRequestsWithMockServer(t, tt.args, []string{http.MethodGet, http.MethodGet}, tt.wantURI, []interface{}{&args{}}, tt.response, tt.responseStatus, tt.want, tt.wantErr, func(client common.DatabricksClient) (interface{}, error) {
				return NewNotebooksAPI(&client).List(tt.args[0].(*args).Path, tt.args[0].(*args).Recursive)
			})
		})
	}
}

func TestNotebooksAPI_Read(t *testing.T) {
	type args struct {
		Path string `json:"path"`
	}
	tests := []struct {
		name           string
		response       string
		args           args
		responseStatus int
		wantURI        string
		want           WorkspaceObjectStatus
		wantErr        bool
	}{
		{
			name: "Read test",
			response: `{
						  "path": "/Users/user@example.com/project/ScalaExampleNotebook",
						  "language": "SCALA",
						  "object_type": "NOTEBOOK",
						  "object_id": 789
						}`,
			args: args{
				Path: "/test/path",
			},
			responseStatus: http.StatusOK,
			want: WorkspaceObjectStatus{
				ObjectID:   789,
				ObjectType: Notebook,
				Path:       "/Users/user@example.com/project/ScalaExampleNotebook",
				Language:   Scala,
			},
			wantURI: "/api/2.0/workspace/get-status?path=%2Ftest%2Fpath",
			wantErr: false,
		},

		{
			name:     "Read test failure",
			response: ``,
			args: args{
				Path: "/test/path",
			},
			responseStatus: http.StatusBadRequest,
			want:           WorkspaceObjectStatus{},
			wantURI:        "/api/2.0/workspace/get-status?path=%2Ftest%2Fpath",
			wantErr:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input args
			qa.AssertRequestWithMockServer(t, &tt.args, http.MethodGet, tt.wantURI, &input, tt.response, tt.responseStatus, tt.want, tt.wantErr, func(client common.DatabricksClient) (interface{}, error) {
				return NewNotebooksAPI(&client).Read(tt.args.Path)
			})
		})
	}
}

func TestNotebooksAPI_Export(t *testing.T) {
	type args struct {
		Path   string       `json:"path"`
		Format ExportFormat `json:"format"`
	}
	tests := []struct {
		name           string
		response       string
		args           args
		responseStatus int
		wantURI        string
		want           string
		wantErr        bool
	}{
		{
			name: "Export test",
			response: `{
						  "content": "Ly8gRGF0YWJyaWNrcyBub3RlYm9vayBzb3VyY2UKMSsx"
						}`,
			args: args{
				Path:   "/test/path",
				Format: DBC,
			},
			responseStatus: http.StatusOK,
			want:           "Ly8gRGF0YWJyaWNrcyBub3RlYm9vayBzb3VyY2UKMSsx",
			wantURI:        "/api/2.0/workspace/export?format=DBC&path=%2Ftest%2Fpath",
			wantErr:        false,
		},
		{
			name:     "Export test failure",
			response: ``,
			args: args{
				Path:   "/test/path",
				Format: DBC,
			},
			responseStatus: http.StatusBadRequest,
			want:           "",
			wantURI:        "/api/2.0/workspace/export?format=DBC&path=%2Ftest%2Fpath",
			wantErr:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var input args
			qa.AssertRequestWithMockServer(t, &tt.args, http.MethodGet, tt.wantURI, &input, tt.response, tt.responseStatus, tt.want, tt.wantErr, func(client common.DatabricksClient) (interface{}, error) {
				return NewNotebooksAPI(&client).Export(tt.args.Path, tt.args.Format)
			})
		})
	}
}

const fileContent string = "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"

func TestAccNotebookCreate(t *testing.T) {
	if _, ok := os.LookupEnv("CLOUD_ENV"); !ok {
		t.Skip("Acceptance tests skipped unless env 'CLOUD_ENV' is set")
	}

	path := "/demo-notebook-rbc"
	format := DBC
	language := Python

	client := common.NewClientFromEnvironment()
	err := NewNotebooksAPI(client).Create(path, fileContent, language, format, false)
	assert.NoError(t, err, err)

	defer func() {
		err := NewNotebooksAPI(client).Delete(path, true)
		assert.NoError(t, err, err)
	}()

	notebookInfo, err := NewNotebooksAPI(client).Read(path)
	assert.NoError(t, err, err)

	_, err = NewNotebooksAPI(client).Export(path, format)
	assert.NoError(t, err, err)
	t.Log(notebookInfo)
}

func TestUri(t *testing.T) {
	uri := "https://sri-e2-test-workspace-3.cloud.databricks.com/api/2.0/workspace/export?format=DBC\u0026path=/demo-notebook-rbc"
	t.Log(url.PathUnescape(uri))
}
func TestAccNotebookUnzip(t *testing.T) {
	if _, ok := os.LookupEnv("CLOUD_ENV"); !ok {
		t.Skip("Acceptance tests skipped unless env 'CLOUD_ENV' is set")
	}

	path := "/demo-notebook-rbc"
	format := DBC

	client := common.NewClientFromEnvironment()

	err := NewNotebooksAPI(client).Create(path, fileContent, Python, format, false)
	assert.NoError(t, err, err)

	defer func() {
		err := NewNotebooksAPI(client).Delete(path, true)
		assert.NoError(t, err, err)
	}()

	export, err := NewNotebooksAPI(client).Export(path, format)
	assert.NoError(t, err, err)
	exportCRC, err := convertBase64ToCheckSum(export)
	assert.NoError(t, err, err)
	expectedCRC, err := convertBase64ToCheckSum(fileContent)
	assert.NoError(t, err, err)
	assert.Equal(t, expectedCRC, exportCRC)
}

func convertZipBytesToCRC(b64 []byte) (string, error) {
	r, err := zip.NewReader(bytes.NewReader(b64), int64(len(b64)))
	if err != nil {
		return "0", err
	}
	var totalSum int64
	for _, f := range r.File {
		if f.FileInfo().IsDir() == false {
			file, err := f.Open()
			if err != nil {
				return "", err
			}
			crc, err := getDBCCheckSumForCommands(file)
			if err != nil {
				return "", err
			}
			totalSum += int64(crc)
		}
	}
	return strconv.Itoa(int(totalSum)), nil
}

func getDBCCheckSumForCommands(fileIO io.Reader) (int, error) {
	var stringBuff bytes.Buffer
	scanner := bufio.NewScanner(fileIO)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		stringBuff.WriteString(scanner.Text())
	}
	jsonString := stringBuff.Bytes()
	var notebook map[string]interface{}
	err := json.Unmarshal(jsonString, &notebook)
	if err != nil {
		return 0, err
	}
	var commandsBuffer bytes.Buffer
	commandsMap := map[int]string{}
	commands := notebook["commands"].([]interface{})
	for _, command := range commands {
		commandsMap[int(command.(map[string]interface{})["position"].(float64))] = command.(map[string]interface{})["command"].(string)
	}
	keys := make([]int, 0, len(commandsMap))
	for k := range commandsMap {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, k := range keys {
		commandsBuffer.WriteString(commandsMap[k])
	}
	return int(crc32.ChecksumIEEE(commandsBuffer.Bytes())), nil
}
