---
name: api-design
description: >-
  Use this skill when designing, implementing, or reviewing REST API endpoints
  in the Go backend. Covers URL conventions, request/response shapes, status
  codes, pagination, filtering, versioning, and OpenAPI documentation.
---

# API Design

## When to Activate

- Adding new API endpoints or modifying existing ones
- Designing request/response payload shapes
- Implementing pagination, filtering, or sorting
- Reviewing API consistency or documentation

---

## URL Conventions

### Rules

1. **Versioned prefix:** `/api/v1/...`
2. **Plural nouns for collections:** `/exams`, `/assignments`, `/sessions`
3. **Nested resources for ownership:** `/exams/:examId/questions`
4. **Actions as sub-resources:** `/sessions/:id/submit` (POST)
5. **No verbs in URLs.** The HTTP method is the verb.

### Standard CRUD

| Method   | URL                          | Action                | Status |
| -------- | ---------------------------- | --------------------- | ------ |
| `GET`    | `/exams`                     | List exams            | 200    |
| `POST`   | `/exams`                     | Create exam           | 201    |
| `GET`    | `/exams/:id`                 | Get exam by ID        | 200    |
| `PUT`    | `/exams/:id`                 | Full update           | 200    |
| `PATCH`  | `/exams/:id`                 | Partial update        | 200    |
| `DELETE` | `/exams/:id`                 | Delete exam           | 204    |

---

## Request/Response Shapes

### Request Body

```json
{
  "title": "Biology 101",
  "subject": "Biology",
  "durationMins": 60,
  "status": "DRAFT"
}
```

**Rules:**
1. `camelCase` field names in JSON.
2. Use Go struct tags: `json:"fieldName"`.
3. Validate with struct tags or a validation library — never trust input.
4. Omit fields with `omitempty` for optional updates.

### Success Response

```json
{
  "id": "uuid-here",
  "title": "Biology 101",
  "createdAt": "2025-01-15T10:30:00Z"
}
```

### List Response (Paginated)

```json
{
  "exams": [...],
  "total": 47,
  "limit": 20,
  "offset": 0
}
```

**Rules:**
1. Wrap array responses in a named key (`"exams"`, not bare `[...]`).
2. Always include `total`, `limit`, `offset` for pagination.

### Error Response

```json
{
  "error": {
    "code": "EXAM_NOT_FOUND",
    "message": "Exam with ID abc123 not found",
    "details": {}
  }
}
```

**Rules:**
1. Consistent error shape across all endpoints.
2. Machine-readable `code`, human-readable `message`.
3. Never leak stack traces, SQL queries, or internal paths.

---

## Status Codes

| Code | When                                                    |
| ---- | ------------------------------------------------------- |
| 200  | Successful read or update                               |
| 201  | Resource created (return the created resource + Location header) |
| 204  | Successful delete (no body)                             |
| 400  | Validation error, malformed request                     |
| 401  | Missing or invalid authentication                       |
| 403  | Authenticated but not authorized                        |
| 404  | Resource not found                                      |
| 409  | Conflict (duplicate email, already submitted)           |
| 422  | Semantic validation failure (valid JSON, invalid logic) |
| 429  | Rate limited                                            |
| 500  | Unexpected server error                                 |

---

## Pagination

```go
type PaginationParams struct {
    Limit  int `query:"limit" validate:"min=1,max=100"`
    Offset int `query:"offset" validate:"min=0"`
}

func (p *PaginationParams) Defaults() {
    if p.Limit == 0 { p.Limit = 20 }
}
```

Query: `GET /exams?limit=20&offset=40&status=PUBLISHED&search=biology`

---

## Filtering & Sorting

```
GET /exams?status=PUBLISHED&sort=createdAt&order=desc
```

**Rules:**
1. Allowlist filterable fields — never pass raw SQL column names.
2. Default sort: `createdAt DESC`.
3. Validate `sort` and `order` against known values.

---

## Handler Pattern

```go
func (h *ExamHandler) GetExams(c echo.Context) error {
    ctx := c.Request().Context()

    var params ExamListParams
    if err := c.Bind(&params); err != nil {
        return echo.NewHTTPError(http.StatusBadRequest, "invalid query parameters")
    }
    params.Defaults()

    exams, total, err := h.service.ListExams(ctx, params)
    if err != nil {
        return mapDomainError(err)
    }

    return c.JSON(http.StatusOK, ExamListResponse{
        Exams:  exams,
        Total:  total,
        Limit:  params.Limit,
        Offset: params.Offset,
    })
}
```

---

## Common Mistakes

| Mistake                               | Fix                                                |
| ------------------------------------- | -------------------------------------------------- |
| Bare JSON arrays `[...]`              | Wrap in object: `{ "exams": [...] }`               |
| Returning 200 for everything          | Use correct status codes per table above            |
| Inconsistent error shapes             | Single `ErrorResponse` struct across all endpoints  |
| No pagination on list endpoints       | Always paginate; default limit=20, max=100          |
| Leaking internal errors               | Map domain errors; never expose SQL or stack traces |
| `snake_case` in JSON                  | Use `camelCase` consistently                        |

---

## Production Checklist

- [ ] All URLs follow `/api/v1/resource` convention
- [ ] Every list endpoint is paginated with `limit`, `offset`, `total`
- [ ] Error response shape is consistent across all endpoints
- [ ] Status codes match the table above
- [ ] Input validated and sanitised before reaching service layer
- [ ] No internal details leaked in error messages
- [ ] Filterable/sortable fields are allowlisted
- [ ] OpenAPI spec exists and stays in sync with implementation
- [ ] CORS configured for frontend origin
- [ ] Rate limiting on auth and write endpoints
