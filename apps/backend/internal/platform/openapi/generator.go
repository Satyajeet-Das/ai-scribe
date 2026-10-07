package openapi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/Satyajeet-Das/ai-scribe/internal/answer"
	"github.com/Satyajeet-Das/ai-scribe/internal/assignment"
	"github.com/Satyajeet-Das/ai-scribe/internal/auth"
	"github.com/Satyajeet-Das/ai-scribe/internal/exam"
	"github.com/Satyajeet-Das/ai-scribe/internal/question"
	"github.com/Satyajeet-Das/ai-scribe/internal/session"
	"github.com/Satyajeet-Das/ai-scribe/internal/user"
)

// SchemaProperty defines an OpenAPI 3.0 schema property.
type SchemaProperty struct {
	Type        string                    `json:"type,omitempty"`
	Format      string                    `json:"format,omitempty"`
	Description string                    `json:"description,omitempty"`
	Example     interface{}               `json:"example,omitempty"`
	Nullable    bool                      `json:"nullable,omitempty"`
	MinLength   *int                      `json:"minLength,omitempty"`
	MaxLength   *int                      `json:"maxLength,omitempty"`
	Minimum     *int                      `json:"minimum,omitempty"`
	Maximum     *int                      `json:"maximum,omitempty"`
	Enum        []string                  `json:"enum,omitempty"`
	Items       *SchemaProperty           `json:"items,omitempty"`
	Ref         string                    `json:"$ref,omitempty"`
	Properties  map[string]SchemaProperty `json:"properties,omitempty"`
}

// SchemaObject defines an OpenAPI 3.0 schema object.
type SchemaObject struct {
	Type        string                    `json:"type"`
	Description string                    `json:"description,omitempty"`
	Required    []string                  `json:"required,omitempty"`
	Properties  map[string]SchemaProperty `json:"properties"`
}

// Parameter defines an OpenAPI 3.0 operation parameter.
type Parameter struct {
	Name        string          `json:"name"`
	In          string          `json:"in"`
	Description string          `json:"description,omitempty"`
	Required    bool            `json:"required,omitempty"`
	Schema      *SchemaProperty `json:"schema,omitempty"`
}

// Operation defines an OpenAPI 3.0 operation.
type Operation struct {
	Tags        []string               `json:"tags,omitempty"`
	Summary     string                 `json:"summary,omitempty"`
	Description string                 `json:"description,omitempty"`
	Security    []map[string][]string  `json:"security,omitempty"`
	Parameters  []Parameter            `json:"parameters,omitempty"`
	RequestBody map[string]interface{} `json:"requestBody,omitempty"`
	Responses   map[string]interface{} `json:"responses,omitempty"`
}

// GetSchemaRegistry returns the mapping of schema names to sample Go instances.
func GetSchemaRegistry() map[string]interface{} {
	return map[string]interface{}{
		"RegisterRequest":               auth.RegisterRequest{},
		"RegisterResponse":              auth.RegisterResponse{},
		"LoginRequest":                  auth.LoginRequest{},
		"LoginResponse":                 auth.LoginResponse{},
		"RefreshResponse":               auth.RefreshResponse{},
		"LogoutResponse":                auth.LogoutResponse{},
		"UserResponse":                  auth.UserResponse{},
		"StudentSearchResult":          user.StudentSearchResult{},
		"User":                          user.User{},
		"CreateExamRequest":             exam.CreateExamRequest{},
		"UpdateExamRequest":             exam.UpdateExamRequest{},
		"ExamResponse":                  exam.ExamResponse{},
		"ExamListResponse":              exam.ExamListResponse{},
		"CreateQuestionRequest":         question.CreateQuestionRequest{},
		"UpdateQuestionRequest":         question.UpdateQuestionRequest{},
		"CreateOptionRequest":           question.CreateOptionRequest{},
		"TeacherQuestionOptionResponse": question.TeacherQuestionOptionResponse{},
		"StudentQuestionOptionResponse": question.StudentQuestionOptionResponse{},
		"TeacherQuestionResponse":       question.TeacherQuestionResponse{},
		"StudentQuestionResponse":       question.StudentQuestionResponse{},
		"CreateAssignmentRequest":          assignment.CreateAssignmentRequest{},
		"BulkAssignRequest":                assignment.BulkAssignRequest{},
		"BulkAssignResponse":               assignment.BulkAssignResponse{},
		"BulkAssignFailure":                assignment.BulkAssignFailure{},
		"CheckAssignmentResponse":          assignment.CheckAssignmentResponse{},
		"StudentAssignedExamResponse":      assignment.StudentAssignedExamResponse{},
		"StudentAssignedExamsListResponse": assignment.StudentAssignedExamsListResponse{},
		"AssignmentResponse":               assignment.AssignmentResponse{},
		"AssignmentListResponse":           assignment.AssignmentListResponse{},
		"StartSessionRequest":              session.StartSessionRequest{},
		"SessionResponse":               session.SessionResponse{},
		"SubmitAnswerRequest":           answer.SubmitAnswerRequest{},
		"AnswerResponse":                answer.AnswerResponse{},
	}
}

// GenerateSchemas inspects Go DTO structs using reflection and generates OpenAPI schemas.
func GenerateSchemas() map[string]SchemaObject {
	schemas := make(map[string]SchemaObject)
	registry := GetSchemaRegistry()

	for name, sample := range registry {
		schema := introspectStruct(reflect.TypeOf(sample))
		schemas[name] = schema
	}

	return schemas
}

func introspectStruct(t reflect.Type) SchemaObject {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	obj := SchemaObject{
		Type:       "object",
		Properties: make(map[string]SchemaProperty),
	}

	var required []string

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Inline embedded/anonymous structs like model.Base
		if field.Anonymous {
			embedded := introspectStruct(field.Type)
			for propName, prop := range embedded.Properties {
				obj.Properties[propName] = prop
			}
			required = append(required, embedded.Required...)
			continue
		}

		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" || jsonTag == "" && !field.IsExported() {
			continue
		}

		parts := strings.Split(jsonTag, ",")
		fieldName := parts[0]
		if fieldName == "" {
			fieldName = strings.ToLower(field.Name[:1]) + field.Name[1:]
		}

		isOmitEmpty := false
		for _, p := range parts[1:] {
			if p == "omitempty" {
				isOmitEmpty = true
				break
			}
		}

		validateTag := field.Tag.Get("validate")
		if strings.Contains(validateTag, "required") && !isOmitEmpty {
			required = append(required, fieldName)
		}

		prop := introspectField(field.Type, field.Tag, fieldName)
		obj.Properties[fieldName] = prop
	}

	if len(required) > 0 {
		obj.Required = required
	}

	return obj
}

func introspectField(t reflect.Type, tag reflect.StructTag, fieldName string) SchemaProperty {
	isPtr := false
	if t.Kind() == reflect.Ptr {
		isPtr = true
		t = t.Elem()
	}

	prop := SchemaProperty{
		Nullable: isPtr,
	}

	// Known concrete types
	if t == reflect.TypeOf(uuid.UUID{}) {
		prop.Type = "string"
		prop.Format = "uuid"
		prop.Example = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
		return prop
	}

	if t == reflect.TypeOf(time.Time{}) {
		prop.Type = "string"
		prop.Format = "date-time"
		return prop
	}

	// Status / Enum types
	switch t.String() {
	case "exam.Status":
		prop.Type = "string"
		prop.Enum = []string{string(exam.StatusDraft), string(exam.StatusPublished), string(exam.StatusArchived)}
		return prop
	case "assignment.Status":
		prop.Type = "string"
		prop.Enum = []string{string(assignment.StatusAssigned), string(assignment.StatusRevoked)}
		return prop
	case "session.Status":
		prop.Type = "string"
		prop.Enum = []string{string(session.StatusPending), string(session.StatusInProgress), string(session.StatusSubmitted), string(session.StatusExpired)}
		return prop
	case "question.Type":
		prop.Type = "string"
		prop.Enum = []string{string(question.TypeMCQ), string(question.TypeEssay), string(question.TypeVoice)}
		return prop
	}

	// Parse validation tag rules
	valTag := tag.Get("validate")
	if valTag != "" {
		rules := strings.Split(valTag, ",")
		for _, r := range rules {
			if r == "email" {
				prop.Format = "email"
			} else if r == "uuid" {
				prop.Format = "uuid"
			} else if strings.HasPrefix(r, "min=") {
				if v, err := strconv.Atoi(strings.TrimPrefix(r, "min=")); err == nil {
					if t.Kind() == reflect.String {
						prop.MinLength = &v
					} else if isNumber(t.Kind()) {
						prop.Minimum = &v
					}
				}
			} else if strings.HasPrefix(r, "max=") {
				if v, err := strconv.Atoi(strings.TrimPrefix(r, "max=")); err == nil {
					if t.Kind() == reflect.String {
						prop.MaxLength = &v
					} else if isNumber(t.Kind()) {
						prop.Maximum = &v
					}
				}
			} else if strings.HasPrefix(r, "oneof=") {
				options := strings.Fields(strings.TrimPrefix(r, "oneof="))
				if len(options) > 0 {
					prop.Enum = options
				}
			}
		}
	}

	switch t.Kind() {
	case reflect.String:
		prop.Type = "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		prop.Type = "integer"
	case reflect.Int64:
		prop.Type = "integer"
		prop.Format = "int64"
	case reflect.Float32, reflect.Float64:
		prop.Type = "number"
	case reflect.Bool:
		prop.Type = "boolean"
	case reflect.Slice, reflect.Array:
		prop.Type = "array"
		elemProp := introspectField(t.Elem(), "", "")
		prop.Items = &elemProp
	case reflect.Struct:
		prop.Type = "object"
		nested := introspectStruct(t)
		prop.Properties = nested.Properties
	default:
		prop.Type = "string"
	}

	// Apply intelligent standard field metadata
	enrichFieldMetadata(&prop, fieldName)

	return prop
}

func isNumber(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

func enrichFieldMetadata(prop *SchemaProperty, name string) {
	switch name {
	case "rollNo":
		prop.Description = "Unique student roll number (mandatory when role is STUDENT, e.g. 23CS001)"
		prop.Example = "23CS001"
	case "assignedCount":
		prop.Description = "Number of students actively assigned to this exam"
		prop.Example = 12
	case "email":
		prop.Description = "User email address"
		prop.Example = "student@university.edu"
	case "password":
		prop.Description = "Account password (min 8 characters)"
		prop.Example = "SecurePassword123!"
	case "firstName":
		prop.Description = "User given name"
		prop.Example = "Alex"
	case "lastName":
		prop.Description = "User family name"
		prop.Example = "Rivera"
	case "durationMins":
		prop.Description = "Duration of the exam in minutes"
		prop.Example = 60
	case "title":
		prop.Description = "Title or name"
		prop.Example = "Biology Midterm Examination"
	case "subject":
		prop.Description = "Academic subject"
		prop.Example = "Biology"
	case "examId":
		prop.Description = "Associated Exam UUID"
	case "studentId":
		prop.Description = "Associated Student UUID"
	case "studentName":
		prop.Description = "Full name of the assigned student"
		prop.Example = "Alex Rivera"
	case "studentRollNo":
		prop.Description = "Roll number of the assigned student"
		prop.Example = "23CS001"
	}
}

// SyncOpenAPISpec synchronizes routes and DTO schemas into openapiPath.
func SyncOpenAPISpec(routes []*echo.Route, openapiPath string) error {
	var doc map[string]interface{}

	content, err := os.ReadFile(openapiPath)
	if err != nil {
		if os.IsNotExist(err) {
			doc = map[string]interface{}{
				"openapi": "3.0.2",
				"info": map[string]interface{}{
					"title":       "AI Exam Scribe API",
					"version":     "1.0.0",
					"description": "Enterprise-grade accessible examination platform",
				},
				"servers": []interface{}{
					map[string]interface{}{
						"url":         "http://localhost:8080",
						"description": "Local Development Server",
					},
				},
				"tags":       []interface{}{},
				"paths":      map[string]interface{}{},
				"components": map[string]interface{}{},
			}
		} else {
			return fmt.Errorf("failed to read openapi file: %w", err)
		}
	} else {
		if err := json.Unmarshal(content, &doc); err != nil {
			return fmt.Errorf("failed to parse existing openapi json: %w", err)
		}
	}

	// 1. Ensure components.schemas exists
	components, ok := doc["components"].(map[string]interface{})
	if !ok || components == nil {
		components = make(map[string]interface{})
		doc["components"] = components
	}

	schemas, ok := components["schemas"].(map[string]interface{})
	if !ok || schemas == nil {
		schemas = make(map[string]interface{})
		components["schemas"] = schemas
	}

	// 2. Generate and merge schemas
	generated := GenerateSchemas()
	for name, genSchema := range generated {
		b, err := json.Marshal(genSchema)
		if err != nil {
			continue
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(b, &parsed); err != nil {
			continue
		}

		if existing, exists := schemas[name].(map[string]interface{}); exists {
			// Update required array to match Go code exactly
			if genReq, ok := parsed["required"]; ok {
				existing["required"] = genReq
			} else {
				delete(existing, "required")
			}

			// Merge properties so we keep hand-crafted field descriptions/examples while updating types
			if existingProps, ok := existing["properties"].(map[string]interface{}); ok {
				if genProps, ok := parsed["properties"].(map[string]interface{}); ok {
					// Add or update generated properties
					for propK, propV := range genProps {
						if _, has := existingProps[propK]; !has {
							existingProps[propK] = propV
						} else {
							if propMap, ok := propV.(map[string]interface{}); ok {
								if existingPropMap, ok := existingProps[propK].(map[string]interface{}); ok {
									if t, ok := propMap["type"]; ok {
										existingPropMap["type"] = t
									}
									if f, ok := propMap["format"]; ok {
										existingPropMap["format"] = f
									}
									if enums, ok := propMap["enum"]; ok {
										existingPropMap["enum"] = enums
									}
									if null, ok := propMap["nullable"]; ok {
										existingPropMap["nullable"] = null
									}
									if desc, ok := propMap["description"]; ok && existingPropMap["description"] == nil {
										existingPropMap["description"] = desc
									}
									if ex, ok := propMap["example"]; ok && existingPropMap["example"] == nil {
										existingPropMap["example"] = ex
									}
								}
							}
						}
					}

					// Remove deprecated properties that no longer exist in Go struct
					for existK := range existingProps {
						if _, stillExists := genProps[existK]; !stillExists {
							delete(existingProps, existK)
						}
					}
				}
			}
		} else {
			schemas[name] = parsed
		}
	}

	// Explicitly document specialized field descriptions and examples
	if examResp, ok := schemas["ExamResponse"].(map[string]interface{}); ok {
		if props, ok := examResp["properties"].(map[string]interface{}); ok {
			if assignedCount, ok := props["assignedCount"].(map[string]interface{}); ok {
				assignedCount["description"] = "Number of students actively assigned to this exam"
				assignedCount["example"] = 15
			}
		}
	}
	if regReq, ok := schemas["RegisterRequest"].(map[string]interface{}); ok {
		if props, ok := regReq["properties"].(map[string]interface{}); ok {
			if rollNo, ok := props["rollNo"].(map[string]interface{}); ok {
				rollNo["description"] = "Unique student roll number (mandatory when role is STUDENT)"
				rollNo["example"] = "23CS001"
			}
		}
	}
	if userResp, ok := schemas["UserResponse"].(map[string]interface{}); ok {
		if props, ok := userResp["properties"].(map[string]interface{}); ok {
			if rollNo, ok := props["rollNo"].(map[string]interface{}); ok {
				rollNo["description"] = "Unique student roll number (null for TEACHER or ADMIN)"
				rollNo["example"] = "23CS001"
			}
		}
	}
	if sessReq, ok := schemas["StartSessionRequest"].(map[string]interface{}); ok {
		if props, ok := sessReq["properties"].(map[string]interface{}); ok {
			if examID, ok := props["examId"].(map[string]interface{}); ok {
				examID["description"] = "Exam UUID to start session for (student must be assigned)"
			}
			if asgnID, ok := props["assignmentId"].(map[string]interface{}); ok {
				asgnID["description"] = "Optional explicit assignment UUID"
			}
		}
	}

	// 3. Ensure paths exists and synchronize routes
	paths, ok := doc["paths"].(map[string]interface{})
	if !ok || paths == nil {
		paths = make(map[string]interface{})
		doc["paths"] = paths
	}

	for _, route := range routes {
		path := route.Path
		method := strings.ToLower(route.Method)

		// Only allow standard HTTP methods
		switch method {
		case "get", "post", "put", "delete", "patch":
		default:
			continue
		}

		// Skip internal/system routes or wildcards
		if path == "" || path == "/*" || strings.HasSuffix(path, "/*") || strings.HasPrefix(path, "/static") || path == "/docs" {
			continue
		}

		// Convert Echo :param syntax to OpenAPI {param} syntax
		openApiPath := convertEchoPathToOpenAPI(path)

		pathItem, ok := paths[openApiPath].(map[string]interface{})
		if !ok || pathItem == nil {
			pathItem = make(map[string]interface{})
			paths[openApiPath] = pathItem
		}

		// Check if operation already exists
		if _, exists := pathItem[method]; !exists {
			// Auto-generate operation
			op := buildDefaultOperation(openApiPath, method)
			pathItem[method] = op
		}
	}

	// Enrich /api/v1/auth/register with clear requestBody example including rollNo
	if regPath, ok := paths["/api/v1/auth/register"].(map[string]interface{}); ok {
		if postOp, ok := regPath["post"].(map[string]interface{}); ok {
			postOp["description"] = "Registers a new user account with role TEACHER, STUDENT, or PROCTOR. For students, rollNo is mandatory."
			postOp["requestBody"] = map[string]interface{}{
				"required": true,
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{
						"schema": map[string]interface{}{
							"$ref": "#/components/schemas/RegisterRequest",
						},
						"example": map[string]interface{}{
							"email":     "student@university.edu",
							"password":  "SecurePass123!",
							"firstName": "Alex",
							"lastName":  "Rivera",
							"role":      "STUDENT",
							"rollNo":    "23CS001",
						},
					},
				},
			}
		}
	}

	// Clean up any stray non-standard paths/methods
	for pKey, pVal := range paths {
		if strings.HasSuffix(pKey, "/*") {
			delete(paths, pKey)
			continue
		}
		if pMap, ok := pVal.(map[string]interface{}); ok {
			for mKey := range pMap {
				switch strings.ToLower(mKey) {
				case "get", "post", "put", "delete", "patch", "head", "options", "parameters":
				default:
					delete(pMap, mKey)
				}
			}
			if len(pMap) == 0 {
				delete(paths, pKey)
			}
		}
	}

	// 4. Format and save
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format openapi JSON: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(openapiPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory for openapi: %w", err)
	}

	if err := os.WriteFile(openapiPath, out, 0644); err != nil {
		return fmt.Errorf("failed to write openapi file: %w", err)
	}

	return nil
}

func convertEchoPathToOpenAPI(path string) string {
	segments := strings.Split(path, "/")
	for i, seg := range segments {
		if strings.HasPrefix(seg, ":") {
			segments[i] = "{" + seg[1:] + "}"
		}
	}
	return strings.Join(segments, "/")
}

func buildDefaultOperation(path, method string) map[string]interface{} {
	tag := "General"
	lowerPath := strings.ToLower(path)

	if strings.Contains(lowerPath, "/auth") {
		tag = "Auth"
	} else if strings.Contains(lowerPath, "/exams") {
		tag = "Exams"
	} else if strings.Contains(lowerPath, "/questions") {
		tag = "Questions"
	} else if strings.Contains(lowerPath, "/assignments") {
		tag = "Assignments"
	} else if strings.Contains(lowerPath, "/sessions") {
		tag = "Sessions"
	} else if strings.Contains(lowerPath, "/answers") {
		tag = "Answers"
	} else if strings.Contains(lowerPath, "/students") {
		tag = "Students"
	} else if strings.Contains(lowerPath, "/status") {
		tag = "Health"
	}

	// Extract path params
	var params []map[string]interface{}
	for _, seg := range strings.Split(path, "/") {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			paramName := seg[1 : len(seg)-1]
			params = append(params, map[string]interface{}{
				"name":     paramName,
				"in":       "path",
				"required": true,
				"schema": map[string]interface{}{
					"type":   "string",
					"format": "uuid",
				},
			})
		}
	}

	summary := fmt.Sprintf("%s %s", strings.ToUpper(method), path)
	if strings.Contains(path, "/assignments") && method == "delete" {
		summary = "Revoke student assignment"
	} else if strings.Contains(path, "/assignments/exam") && method == "get" {
		summary = "Get assignments for exam (alias)"
	} else if strings.Contains(path, "/students/search") && method == "get" {
		summary = "Search students by roll number or name"
		params = append(params, map[string]interface{}{
			"name":        "q",
			"in":          "query",
			"description": "Roll number or student name search query",
			"required":    true,
			"schema": map[string]interface{}{
				"type": "string",
			},
		}, map[string]interface{}{
			"name":        "limit",
			"in":          "query",
			"description": "Maximum number of results to return",
			"schema": map[string]interface{}{
				"type":    "integer",
				"default": 10,
			},
		})
	}

	successCode := "200"
	switch method {
	case "post":
		successCode = "201"
	case "delete":
		successCode = "204"
	}

	op := map[string]interface{}{
		"tags":        []string{tag},
		"summary":     summary,
		"description": fmt.Sprintf("Automatically generated operation for %s %s", strings.ToUpper(method), path),
		"security": []interface{}{
			map[string]interface{}{
				"bearerAuth": []string{},
			},
		},
		"responses": map[string]interface{}{
			successCode: map[string]interface{}{
				"description": "Successful operation",
			},
			"400": map[string]interface{}{
				"$ref": "#/components/responses/BadRequest",
			},
			"401": map[string]interface{}{
				"$ref": "#/components/responses/Unauthorized",
			},
			"403": map[string]interface{}{
				"$ref": "#/components/responses/Forbidden",
			},
			"404": map[string]interface{}{
				"$ref": "#/components/responses/NotFound",
			},
		},
	}
	var reqSchemaRef string
	switch {
	case path == "/api/v1/auth/register":
		reqSchemaRef = "#/components/schemas/RegisterRequest"
	case path == "/api/v1/auth/login":
		reqSchemaRef = "#/components/schemas/LoginRequest"
	case path == "/api/v1/exams" && method == "post":
		reqSchemaRef = "#/components/schemas/CreateExamRequest"
	case strings.HasPrefix(path, "/api/v1/exams/") && method == "put":
		reqSchemaRef = "#/components/schemas/UpdateExamRequest"
	case strings.HasSuffix(path, "/questions") && method == "post":
		reqSchemaRef = "#/components/schemas/CreateQuestionRequest"
	case (path == "/api/v1/assignments/bulk" || strings.HasSuffix(path, "/assignments/bulk")) && method == "post":
		reqSchemaRef = "#/components/schemas/BulkAssignRequest"
	case (path == "/api/v1/assignments" || strings.HasSuffix(path, "/assignments")) && method == "post":
		reqSchemaRef = "#/components/schemas/CreateAssignmentRequest"
	case path == "/api/v1/sessions" && method == "post":
		reqSchemaRef = "#/components/schemas/StartSessionRequest"
	case strings.HasSuffix(path, "/answers") && method == "post":
		reqSchemaRef = "#/components/schemas/SubmitAnswerRequest"
	}

	if reqSchemaRef != "" {
		bodyContent := map[string]interface{}{
			"schema": map[string]interface{}{
				"$ref": reqSchemaRef,
			},
		}
		if path == "/api/v1/auth/register" {
			bodyContent["example"] = map[string]interface{}{
				"email":     "student@university.edu",
				"password":  "SecurePass123!",
				"firstName": "Alex",
				"lastName":  "Rivera",
				"role":      "STUDENT",
				"rollNo":    "23CS001",
			}
		}
		op["requestBody"] = map[string]interface{}{
			"required": true,
			"content": map[string]interface{}{
				"application/json": bodyContent,
			},
		}
	}

	if len(params) > 0 {
		op["parameters"] = params
	}

	return op
}
