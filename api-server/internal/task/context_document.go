package task

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrContextDocumentNotFound = errors.New("context document not found")

type ContextDocumentProvider interface {
	GetContextDocument(id int64) (ContextDocument, error)
	UpdateContextDocument(id int64, update ContextDocumentUpdate) (ContextDocument, error)
}

type contextDocumentProvider struct {
	mu               sync.RWMutex
	contextDocuments map[int64]*ContextDocument
}

var _ ContextDocumentProvider = &contextDocumentProvider{}

func NewContextDocumentProvider() *contextDocumentProvider {
	docs := prefilledContextDocuments()

	contextDocuments := make(map[int64]*ContextDocument, len(docs))
	for i := range docs {
		doc := docs[i]
		contextDocuments[doc.Id] = &doc
	}

	return &contextDocumentProvider{
		contextDocuments: contextDocuments,
	}
}

type JobState struct {
	Id      int64
	IssueId int64

	Type  string
	State string
}

type ContextDocument struct {
	Id          int64
	IssueId     int64
	Title       string
	Description string
	Labels      []string

	Members   []string
	Assignees []string
	Reporter  string

	Keywords     []string
	CodeFindings []CodeFinding
	Learnings    []Learning

	Spec string
}

type CodeFinding struct {
	Finding          string
	Sources          []string
	SuggestedActions []string
}

// type Spec struct {
// }

type Learning struct {
	Text      string
	UpdatedBy string
	UpdatedAt time.Time
}

type ContextDocumentUpdate struct {
	IssueId     *int64
	Title       *string
	Description *string
	Labels      *[]string

	Members   *[]string
	Assignees *[]string
	Reporter  *string

	Keywords     *[]string
	CodeFindings *[]CodeFinding
	Learnings    *[]Learning
	Spec         *string
}

func (c *contextDocumentProvider) GetContextDocument(id int64) (ContextDocument, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	doc, ok := c.contextDocuments[id]
	if !ok || doc == nil {
		return ContextDocument{}, fmt.Errorf("%w: id=%d", ErrContextDocumentNotFound, id)
	}

	return *doc, nil
}

func (c *contextDocumentProvider) UpdateContextDocument(id int64, update ContextDocumentUpdate) (ContextDocument, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	doc, ok := c.contextDocuments[id]
	if !ok || doc == nil {
		return ContextDocument{}, fmt.Errorf("%w: id=%d", ErrContextDocumentNotFound, id)
	}

	if update.IssueId != nil {
		doc.IssueId = *update.IssueId
	}
	if update.Title != nil {
		doc.Title = *update.Title
	}
	if update.Description != nil {
		doc.Description = *update.Description
	}
	if update.Labels != nil {
		doc.Labels = *update.Labels
	}
	if update.Members != nil {
		doc.Members = *update.Members
	}
	if update.Assignees != nil {
		doc.Assignees = *update.Assignees
	}
	if update.Reporter != nil {
		doc.Reporter = *update.Reporter
	}
	if update.Keywords != nil {
		doc.Keywords = *update.Keywords
	}
	if update.CodeFindings != nil {
		doc.CodeFindings = *update.CodeFindings
	}
	if update.Learnings != nil {
		doc.Learnings = *update.Learnings
	}
	if update.Spec != nil {
		doc.Spec = *update.Spec
	}

	return *doc, nil
}

func prefilledContextDocuments() []ContextDocument {

	return []ContextDocument{
		{
			Id:           1,
			IssueId:      100,
			Title:        "Allow partial claim payout for accidental coverage",
			Description:  "Customers should be able to take partial payouts for approved accidental claims instead of full settlement.",
			Labels:       []string{"claims", "payouts", "relay=scoping"},
			Members:      []string{"dev_1", "lead_1", "pm_1", "dev_2"},
			Assignees:    []string{"dev_1", "dev_2"},
			Reporter:     "pm_1",
			Keywords:     []string{"partial payout", "claims", "settlement", "coverage"},
			CodeFindings: []CodeFinding{},
			Learnings:    []Learning{},
			Spec:         "",
		},
	}
}
