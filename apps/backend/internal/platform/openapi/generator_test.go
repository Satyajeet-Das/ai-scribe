package openapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateSchemas(t *testing.T) {
	schemas := GenerateSchemas()

	// 1. Verify RegisterRequest & UserResponse has rollNo
	regReq, ok := schemas["RegisterRequest"]
	require.True(t, ok, "RegisterRequest schema must exist")
	rollNo, hasRollNo := regReq.Properties["rollNo"]
	assert.True(t, hasRollNo, "RegisterRequest must have rollNo property")
	assert.Equal(t, "string", rollNo.Type)
	assert.NotEmpty(t, rollNo.Description, "rollNo must have a description")
	assert.Equal(t, "23CS001", rollNo.Example, "rollNo must have an example")
	assert.True(t, rollNo.Nullable, "rollNo must be nullable/optional in struct")

	userResp, ok := schemas["UserResponse"]
	require.True(t, ok, "UserResponse schema must exist")
	userRollNo, hasUserRollNo := userResp.Properties["rollNo"]
	assert.True(t, hasUserRollNo, "UserResponse must have rollNo property")
	assert.Equal(t, "string", userRollNo.Type)

	// 2. Verify ExamResponse has assignedCount
	examResp, ok := schemas["ExamResponse"]
	require.True(t, ok, "ExamResponse schema must exist")
	assignedCount, hasAssignedCount := examResp.Properties["assignedCount"]
	assert.True(t, hasAssignedCount, "ExamResponse must have assignedCount property")
	assert.Equal(t, "integer", assignedCount.Type)
	assert.NotEmpty(t, assignedCount.Description)

	// 3. Verify AssignmentResponse has studentName and studentRollNo
	asgnResp, ok := schemas["AssignmentResponse"]
	require.True(t, ok, "AssignmentResponse schema must exist")
	nameProp, hasName := asgnResp.Properties["studentName"]
	assert.True(t, hasName, "AssignmentResponse must have studentName property")
	assert.Equal(t, "string", nameProp.Type)

	rollProp, hasRoll := asgnResp.Properties["studentRollNo"]
	assert.True(t, hasRoll, "AssignmentResponse must have studentRollNo property")
	assert.Equal(t, "string", rollProp.Type)

	// 4. Verify StartSessionRequest has examId
	sessReq, ok := schemas["StartSessionRequest"]
	require.True(t, ok, "StartSessionRequest schema must exist")
	examIDProp, hasExamID := sessReq.Properties["examId"]
	assert.True(t, hasExamID, "StartSessionRequest must have examId property")
	assert.Equal(t, "uuid", examIDProp.Format)

	// 5. Verify StudentSearchResult has rollNo, name, email, id
	studentSearch, ok := schemas["StudentSearchResult"]
	require.True(t, ok, "StudentSearchResult schema must exist")
	assert.Contains(t, studentSearch.Properties, "id")
	assert.Contains(t, studentSearch.Properties, "rollNo")
	assert.Contains(t, studentSearch.Properties, "name")
	assert.Contains(t, studentSearch.Properties, "email")
}

func TestOpenAPISpec_EveryGoDTOFieldIsRepresented(t *testing.T) {
	registry := GetSchemaRegistry()
	generated := GenerateSchemas()

	for schemaName, sample := range registry {
		schema, exists := generated[schemaName]
		require.Truef(t, exists, "Schema %s must exist in generated schemas", schemaName)

		valType := reflect.TypeOf(sample)
		if valType.Kind() == reflect.Ptr {
			valType = valType.Elem()
		}

		// Inspect all exported fields with json tags
		for i := 0; i < valType.NumField(); i++ {
			field := valType.Field(i)
			if field.Anonymous {
				continue
			}
			tag := field.Tag.Get("json")
			if tag == "-" || tag == "" && !field.IsExported() {
				continue
			}
			parts := strings.Split(tag, ",")
			propName := parts[0]
			if propName == "" {
				propName = strings.ToLower(field.Name[:1]) + field.Name[1:]
			}

			_, hasProp := schema.Properties[propName]
			assert.Truef(t, hasProp, "Schema %s must have property %s corresponding to Go struct field %s", schemaName, propName, field.Name)
		}
	}
}

func TestSyncOpenAPISpec(t *testing.T) {
	tmpDir := t.TempDir()
	specFile := filepath.Join(tmpDir, "openapi.json")

	routes := []*echo.Route{
		{
			Method: "GET",
			Path:   "/api/v1/exams",
		},
		{
			Method: "GET",
			Path:   "/api/v1/exams/:id",
		},
		{
			Method: "DELETE",
			Path:   "/api/v1/assignments/:id",
		},
		{
			Method: "GET",
			Path:   "/api/v1/students/search",
		},
		{
			Method: "POST",
			Path:   "/api/v1/auth/register",
		},
		{
			Method: "ECHO_ROUTE_NOT_FOUND",
			Path:   "/api/v1/exams/*",
		},
	}

	err := SyncOpenAPISpec(routes, specFile)
	require.NoError(t, err)

	content, err := os.ReadFile(specFile)
	require.NoError(t, err)
	jsonStr := string(content)

	// Ensure valid paths exist
	assert.Contains(t, jsonStr, "/api/v1/exams/{id}")
	assert.Contains(t, jsonStr, "/api/v1/assignments/{id}")
	assert.Contains(t, jsonStr, "/api/v1/students/search")
	assert.Contains(t, jsonStr, "/api/v1/auth/register")

	// Ensure invalid/wildcard paths were omitted
	assert.NotContains(t, jsonStr, "ECHO_ROUTE_NOT_FOUND")
	assert.NotContains(t, jsonStr, "/api/v1/exams/*")

	// Ensure rollNo example is in register operation
	assert.Contains(t, jsonStr, `"rollNo": "23CS001"`)
}

func TestCheckedInOpenAPISpec_IsCompleteAndSynced(t *testing.T) {
	// Reads actual static/openapi.json to verify it has required fields
	candidates := []string{
		"static/openapi.json",
		"../../static/openapi.json",
		"../../../static/openapi.json",
		"apps/backend/static/openapi.json",
	}
	var path string
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			path = c
			break
		}
	}
	require.NotEmpty(t, path, "static/openapi.json must exist in repository")

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(content, &doc))

	components := doc["components"].(map[string]interface{})
	schemas := components["schemas"].(map[string]interface{})

	// Check RegisterRequest has rollNo
	regReq := schemas["RegisterRequest"].(map[string]interface{})
	regProps := regReq["properties"].(map[string]interface{})
	assert.Contains(t, regProps, "rollNo", "RegisterRequest in static/openapi.json MUST contain rollNo")

	// Check UserResponse has rollNo
	userResp := schemas["UserResponse"].(map[string]interface{})
	userProps := userResp["properties"].(map[string]interface{})
	assert.Contains(t, userProps, "rollNo", "UserResponse in static/openapi.json MUST contain rollNo")

	// Check ExamResponse has assignedCount
	examResp := schemas["ExamResponse"].(map[string]interface{})
	examProps := examResp["properties"].(map[string]interface{})
	assert.Contains(t, examProps, "assignedCount", "ExamResponse in static/openapi.json MUST contain assignedCount")

	// Check AssignmentResponse has studentName and studentRollNo
	asgnResp := schemas["AssignmentResponse"].(map[string]interface{})
	asgnProps := asgnResp["properties"].(map[string]interface{})
	assert.Contains(t, asgnProps, "studentName", "AssignmentResponse in static/openapi.json MUST contain studentName")
	assert.Contains(t, asgnProps, "studentRollNo", "AssignmentResponse in static/openapi.json MUST contain studentRollNo")
}
