package openapi

// The exam document endpoints are written out in full here, and re-applied on
// every sync, so the contract cannot drift from the code the way a generated
// stub can. Everything is a plain map so it merges into the existing document.

const documentsTag = "Exam Documents"

type obj = map[string]interface{}

func schemaRef(name string) obj { return obj{"$ref": "#/components/schemas/" + name} }

func jsonContent(schema string) obj {
	return obj{"application/json": obj{"schema": schemaRef(schema)}}
}

func apiError(description string) obj {
	return obj{"description": description, "content": jsonContent("ApiError")}
}

func uuidParam(name, description string) obj {
	return obj{"name": name, "in": "path", "required": true, "description": description,
		"schema": obj{"type": "string", "format": "uuid"}}
}

func versionQuery() obj {
	return obj{"name": "expectedDraftVersion", "in": "query", "required": true,
		"description": "The draft version the caller last read. A different current version is refused with 409 STALE_VERSION.",
		"schema":      obj{"type": "integer", "minimum": 0}}
}

func jsonBody(schema string, required bool, example interface{}) obj {
	media := obj{"schema": schemaRef(schema)}
	if example != nil {
		media["example"] = example
	}
	return obj{"required": required, "content": obj{"application/json": media}}
}

// errors common to every endpoint under an exam.
func commonErrors(extra obj) obj {
	r := obj{
		"401": obj{"$ref": "#/components/responses/Unauthorized"},
		"403": apiError("The caller is not a teacher or administrator."),
		"404": apiError("The exam or document does not exist, or belongs to another teacher. The two cases are not told apart."),
	}
	for k, v := range extra {
		r[k] = v
	}
	return r
}

func operation(summary, description string, params []interface{}, body obj, responses obj) obj {
	op := obj{
		"tags":        []string{documentsTag},
		"summary":     summary,
		"description": description,
		"security":    []interface{}{obj{"bearerAuth": []string{}}},
		"parameters":  params,
		"responses":   responses,
	}
	if body != nil {
		op["requestBody"] = body
	}
	return op
}

const (
	pathDocs   = "/api/v1/exams/{exam_id}/documents"
	pathDoc    = pathDocs + "/{document_id}"
	pathDraft  = pathDoc + "/draft"
	pathDraftQ = pathDraft + "/questions"
)

func enrichDocumentOperations(doc obj, paths obj) {
	components, _ := doc["components"].(obj)
	if components == nil {
		components = obj{}
		doc["components"] = components
	}
	schemas, _ := components["schemas"].(obj)
	if schemas == nil {
		schemas = obj{}
		components["schemas"] = schemas
	}
	schemas["ApiError"] = obj{
		"type":        "object",
		"description": "The error body every endpoint returns. code is stable and meant for programs; message is meant for people.",
		"required":    []string{"code", "message", "status"},
		"properties": obj{
			"code":     obj{"type": "string", "example": "STALE_VERSION"},
			"message":  obj{"type": "string"},
			"status":   obj{"type": "integer", "example": 409},
			"override": obj{"type": "boolean", "description": "True when message is safe to show to the user as it is."},
			"errors": obj{"type": "array", "description": "Field-level detail, for validation errors and for DRAFT_INCOMPLETE.",
				"items": obj{"type": "object", "properties": obj{"field": obj{"type": "string"}, "error": obj{"type": "string"}}}},
		},
	}

	tags, _ := doc["tags"].([]interface{})
	found := false
	for _, t := range tags {
		if m, ok := t.(obj); ok && m["name"] == documentsTag {
			found = true
		}
	}
	if !found {
		doc["tags"] = append(tags, obj{"name": documentsTag,
			"description": "Upload an exam PDF, watch it being read, correct the extracted questions and approve them into a draft exam"})
	}

	exam := uuidParam("exam_id", "The exam the document belongs to. It must be a draft exam owned by the caller (administrators may use any).")
	document := uuidParam("document_id", "The document.")
	question := uuidParam("question_id", "The draft question.")

	set := func(path, method string, op obj) {
		item, _ := paths[path].(obj)
		if item == nil {
			item = obj{}
			paths[path] = item
		}
		item[method] = op
	}

	const flow = " Extracted content is only ever a draft: it reaches the exam's questions through POST …/approve and nowhere else."

	set(pathDocs, "post", operation(
		"Upload an exam document",
		"Uploads a PDF (multipart/form-data, field `file`) and queues it for reading. Returns 202 with the document in status QUEUED; poll `GET …/documents/{document_id}` for progress. "+
			"The file is checked for size, PDF signature, structure, encryption and page count before anything is stored. It is kept privately under a server-generated name, and the client's filename is only a label."+flow+
			"\n\nErrors: `EXAM_NOT_DRAFT` (409), `DOCUMENT_ALREADY_UPLOADED` (409, `errors[0].error` is the existing document id), `TOO_MANY_DOCUMENTS` (409), `PAYLOAD_TOO_LARGE` (413), "+
			"`UNSUPPORTED_MEDIA_TYPE` (415), `INVALID_PDF` (422; `errors[0].error` is NOT_A_PDF, MALFORMED, ENCRYPTED, TOO_MANY_PAGES or NO_PAGES), `EXTRACTION_UNAVAILABLE`, `QUEUE_UNAVAILABLE`, `STORAGE_UNAVAILABLE`, `BUSY` (503). Rate limited per user.",
		[]interface{}{exam},
		obj{"required": true, "content": obj{"multipart/form-data": obj{"schema": obj{
			"type": "object", "required": []string{"file"},
			"properties": obj{"file": obj{"type": "string", "format": "binary", "description": "The exam paper as a PDF."}},
		}}}},
		commonErrors(obj{
			"202": obj{"description": "Accepted and queued.", "content": jsonContent("DocumentResponse")},
			"400": apiError("No `file` part, or the body is not readable multipart."),
			"409": apiError("EXAM_NOT_DRAFT, DOCUMENT_ALREADY_UPLOADED or TOO_MANY_DOCUMENTS."),
			"413": apiError("PAYLOAD_TOO_LARGE: over the size limit. The upload is cut off rather than read to the end."),
			"415": apiError("UNSUPPORTED_MEDIA_TYPE: not multipart, or the file is not a PDF."),
			"422": apiError("INVALID_PDF: damaged, truncated, password protected, empty or too many pages."),
			"429": apiError("Too many uploads."),
			"503": apiError("EXTRACTION_UNAVAILABLE, QUEUE_UNAVAILABLE, STORAGE_UNAVAILABLE or BUSY."),
		})))

	set(pathDocs, "get", operation("List an exam's documents", "Newest first. Page-level detail is left out; fetch one document for it.",
		[]interface{}{exam}, nil, commonErrors(obj{"200": obj{"description": "The documents.", "content": jsonContent("DocumentListResponse")}})))

	set(pathDoc, "get", operation("Get a document and its processing status",
		"Status is one of QUEUED, PROCESSING, REVIEW_REQUIRED, COMPLETED, FAILED, CANCELLED. While PROCESSING, `stage` and `progress` say where it is. "+
			"A FAILED document has a `failureCode` and a `failureMessage` written by this server (never the provider's words). `pages` says what was found on each page, including header and footer lines that were set aside and pages that could not be read.",
		[]interface{}{exam, document}, nil, commonErrors(obj{"200": obj{"description": "The document.", "content": jsonContent("DocumentResponse")}})))

	set(pathDoc+"/file", "get", operation("Download the original PDF",
		"Streams the uploaded file to the exam's owner as an attachment. Never public, never cached.",
		[]interface{}{exam, document}, nil, commonErrors(obj{
			"200": obj{"description": "The PDF.", "content": obj{"application/pdf": obj{"schema": obj{"type": "string", "format": "binary"}}}},
			"503": apiError("STORAGE_UNAVAILABLE."),
		})))

	set(pathDoc, "delete", operation("Delete a document",
		"Removes the record and the stored file, whatever the status. Questions already approved into the exam are kept; only their link to the document is cleared. A running extraction stops.",
		[]interface{}{exam, document}, nil, commonErrors(obj{"204": obj{"description": "Deleted."}})))

	set(pathDoc+"/cancel", "post", operation("Cancel processing",
		"Cancels a QUEUED or PROCESSING document. A running job notices within seconds and stops. Cancelling an already cancelled document succeeds; any other status is 409 INVALID_STATE.",
		[]interface{}{exam, document}, nil, commonErrors(obj{
			"200": obj{"description": "The cancelled document.", "content": jsonContent("DocumentResponse")},
			"409": apiError("INVALID_STATE."),
		})))

	set(pathDoc+"/retry", "post", operation("Retry processing",
		"Runs a FAILED or CANCELLED document again as a new attempt. A document with a draft waiting for review is only re-run with `discardDraft: true` and the `expectedDraftVersion` the caller last saw; its draft is then replaced, never added to. The body may be omitted.",
		[]interface{}{exam, document},
		jsonBody("RetryDocumentRequest", false, obj{"discardDraft": true, "expectedDraftVersion": 3}),
		commonErrors(obj{
			"202": obj{"description": "Queued again.", "content": jsonContent("DocumentResponse")},
			"409": apiError("INVALID_STATE, STALE_VERSION or DOCUMENT_ALREADY_UPLOADED."),
			"503": apiError("EXTRACTION_UNAVAILABLE or QUEUE_UNAVAILABLE."),
		})))

	set(pathDraft, "get", operation("Get the extracted draft",
		"Questions in order, each with its source pages, a confidence, warnings and (for multiple choice) the options with the labels the paper printed. Also the paper's general instructions and any page content that could not be attached to a question. "+
			"Available once the document is REVIEW_REQUIRED, and read-only after COMPLETED; 409 INVALID_STATE before that. No correct answers are ever inferred: `isCorrect` is only what a teacher set. `draftVersion` must be sent back with every change.",
		[]interface{}{exam, document}, nil, commonErrors(obj{
			"200": obj{"description": "The draft.", "content": jsonContent("DraftResponse")},
			"409": apiError("INVALID_STATE: no draft yet."),
		})))

	editErrors := "\n\nErrors: `VALIDATION_ERROR` (400, with `errors[].field`), `STALE_VERSION`, `INVALID_STATE`, `ALREADY_APPROVED`, `DRAFT_TOO_LARGE` (409)."

	set(pathDraftQ, "post", operation("Add a question to the draft",
		"Adds a question the extraction missed, at `position` or last. The new question is marked as written by a teacher."+editErrors,
		[]interface{}{exam, document},
		jsonBody("AddDraftQuestionRequest", true, obj{"expectedDraftVersion": 3, "position": 2, "question": obj{
			"numberLabel": "3", "type": "MCQ", "text": "Which of these is a vector quantity?", "points": 2,
			"options": []obj{{"label": "A", "text": "Mass"}, {"label": "B", "text": "Velocity"}}}}),
		commonErrors(obj{
			"201": obj{"description": "The new question and the draft's new version.", "content": jsonContent("DraftQuestionResultResponse")},
			"400": obj{"$ref": "#/components/responses/BadRequest"},
			"409": apiError("STALE_VERSION, INVALID_STATE, ALREADY_APPROVED or DRAFT_TOO_LARGE."),
		})))

	set(pathDraftQ+"/{question_id}", "put", operation("Replace a draft question",
		"Replaces everything a teacher can edit in the question: the request is the complete state wanted, not a patch. "+
			"The source pages, confidence and the warnings the system raised are kept. Blocking warnings stay unacknowledged unless their codes are listed in `acknowledgedWarnings` in this same request. "+
			"A diagram description is marked as the teacher's as soon as the teacher changes it, and counts as confirmed only when `confirmed` is true."+editErrors,
		[]interface{}{exam, document, question},
		jsonBody("ReplaceDraftQuestionRequest", true, obj{"expectedDraftVersion": 3, "question": obj{
			"numberLabel": "1", "type": "MCQ", "text": "What is the SI unit of force?", "points": 2,
			"options":              []obj{{"label": "A", "text": "Newton", "isCorrect": true}, {"label": "B", "text": "Joule"}},
			"acknowledgedWarnings": []string{"UNVERIFIED_TEXT"}}}),
		commonErrors(obj{
			"200": obj{"description": "The question as saved and the draft's new version.", "content": jsonContent("DraftQuestionResultResponse")},
			"400": obj{"$ref": "#/components/responses/BadRequest"},
			"409": apiError("STALE_VERSION, INVALID_STATE or ALREADY_APPROVED."),
		})))

	set(pathDraftQ+"/{question_id}", "delete", operation("Remove a draft question",
		"Removes the question and closes the gap in the order. Returns the whole draft."+editErrors,
		[]interface{}{exam, document, question, versionQuery()}, nil, commonErrors(obj{
			"200": obj{"description": "The draft after the removal.", "content": jsonContent("DraftResponse")},
			"400": obj{"$ref": "#/components/responses/BadRequest"},
			"409": apiError("STALE_VERSION, INVALID_STATE or ALREADY_APPROVED."),
		})))

	set(pathDraft+"/reorder", "put", operation("Reorder the draft",
		"`questionIds` must list every draft question exactly once, in the new order."+editErrors,
		[]interface{}{exam, document},
		jsonBody("ReorderDraftRequest", true, obj{"expectedDraftVersion": 4, "questionIds": []string{"…", "…"}}),
		commonErrors(obj{
			"200": obj{"description": "The draft in its new order.", "content": jsonContent("DraftResponse")},
			"400": obj{"$ref": "#/components/responses/BadRequest"},
			"409": apiError("STALE_VERSION, INVALID_STATE or ALREADY_APPROVED."),
		})))

	set(pathDoc+"/approve", "post", operation("Approve the draft into the exam",
		"The one and only way extracted content reaches the exam. Copies the reviewed draft into the exam's questions, after the ones already there, in one transaction: all of it or none of it. "+
			"The exam must still be a draft; it is locked for the duration, so it cannot be published half-way. Publishing and assigning are not done here: use the exam's own publish endpoint afterwards. "+
			"Approval needs a complete draft: marks set on every question, at least two options on every multiple-choice question, every diagram described and confirmed, and every blocking warning acknowledged. "+
			"All problems are returned together as 409 `DRAFT_INCOMPLETE` with `errors[].field` such as `questions[2].points`. Each question's text is composed from its instructions, the question, its tables (as rows of text) and its confirmed diagram descriptions, because that single text is what a candidate hears."+
			"\n\nErrors: `VALIDATION_ERROR` (400), `DRAFT_INCOMPLETE`, `STALE_VERSION`, `ALREADY_APPROVED`, `EXAM_NOT_DRAFT`, `INVALID_STATE`, `CONCURRENT_CHANGE` (409).",
		[]interface{}{exam, document},
		jsonBody("ApproveDraftRequest", true, obj{"expectedDraftVersion": 5}),
		commonErrors(obj{
			"200": obj{"description": "The completed document and the questions created.", "content": jsonContent("ApproveResponse")},
			"400": obj{"$ref": "#/components/responses/BadRequest"},
			"409": apiError("DRAFT_INCOMPLETE, STALE_VERSION, ALREADY_APPROVED, EXAM_NOT_DRAFT, INVALID_STATE or CONCURRENT_CHANGE."),
		})))
}
