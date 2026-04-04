package registryprivate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/ssovich/diasoft-gateway/internal/privateapi"
)

type Client struct {
	baseURL      string
	serviceToken string
	httpClient   *http.Client
}

func NewClient(baseURL, serviceToken string, timeout time.Duration) *Client {
	return &Client{
		baseURL:      strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		serviceToken: strings.TrimSpace(serviceToken),
		httpClient:   &http.Client{Timeout: timeout},
	}
}

func (c *Client) ListUniversityDiplomas(ctx context.Context, universityID, search, status string, page int) (privateapi.UniversityDiplomaList, error) {
	query := url.Values{}
	query.Set("universityId", universityID)
	if strings.TrimSpace(search) != "" {
		query.Set("search", strings.TrimSpace(search))
	}
	if strings.TrimSpace(status) != "" {
		query.Set("status", strings.TrimSpace(status))
	}
	query.Set("page", strconv.Itoa(page))

	var response registryDiplomaListResponse
	if err := c.doJSON(ctx, http.MethodGet, "/internal/gateway/university/diplomas?"+query.Encode(), nil, "", &response); err != nil {
		return privateapi.UniversityDiplomaList{}, err
	}

	items := make([]privateapi.UniversityDiplomaItem, 0, len(response.Items))
	for _, item := range response.Items {
		items = append(items, mapUniversityDiploma(item))
	}
	return privateapi.UniversityDiplomaList{Items: items, Total: response.Total}, nil
}

func (c *Client) UploadUniversityDiplomas(ctx context.Context, universityID, filename, contentType string, content []byte) (privateapi.ImportAccepted, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return privateapi.ImportAccepted{}, fmt.Errorf("create multipart file: %w", err)
	}
	if _, err := part.Write(content); err != nil {
		return privateapi.ImportAccepted{}, fmt.Errorf("write multipart file: %w", err)
	}
	if contentType != "" {
		_ = writer.WriteField("contentType", contentType)
	}
	if err := writer.Close(); err != nil {
		return privateapi.ImportAccepted{}, fmt.Errorf("close multipart writer: %w", err)
	}

	query := url.Values{}
	query.Set("universityId", universityID)

	var response registryImportAcceptedResponse
	if err := c.doJSON(ctx, http.MethodPost, "/internal/gateway/university/imports?"+query.Encode(), bytes.NewReader(body.Bytes()), writer.FormDataContentType(), &response); err != nil {
		return privateapi.ImportAccepted{}, err
	}
	return privateapi.ImportAccepted{JobID: response.JobID, Status: response.Status}, nil
}

func (c *Client) GetUniversityImport(ctx context.Context, universityID, jobID string) (privateapi.ImportStatus, error) {
	query := url.Values{}
	query.Set("universityId", universityID)

	var response registryImportJobResponse
	if err := c.doJSON(ctx, http.MethodGet, "/internal/gateway/university/imports/"+jobID+"?"+query.Encode(), nil, "", &response); err != nil {
		return privateapi.ImportStatus{}, err
	}
	return privateapi.ImportStatus{
		JobID:     response.ID,
		Status:    response.Status,
		Total:     response.TotalRows,
		Imported:  response.ProcessedRows,
		Failed:    response.FailedRows,
		CreatedAt: response.CreatedAt,
		UpdatedAt: response.UpdatedAt,
	}, nil
}

func (c *Client) GetUniversityImportErrors(ctx context.Context, universityID, jobID string) ([]privateapi.ImportError, error) {
	query := url.Values{}
	query.Set("universityId", universityID)

	var response registryImportErrorsResponse
	if err := c.doJSON(ctx, http.MethodGet, "/internal/gateway/university/imports/"+jobID+"/errors?"+query.Encode(), nil, "", &response); err != nil {
		return nil, err
	}
	errors := make([]privateapi.ImportError, 0, len(response.Errors))
	for _, item := range response.Errors {
		errors = append(errors, privateapi.ImportError{Row: item.Row, Message: item.Message})
	}
	return errors, nil
}

func (c *Client) RevokeUniversityDiploma(ctx context.Context, universityID, diplomaID, reason string) error {
	query := url.Values{}
	query.Set("universityId", universityID)

	payload, err := json.Marshal(map[string]string{"reason": reason})
	if err != nil {
		return fmt.Errorf("marshal revoke payload: %w", err)
	}
	return c.doJSON(ctx, http.MethodPost, "/internal/gateway/university/diplomas/"+diplomaID+"/revoke?"+query.Encode(), bytes.NewReader(payload), "application/json", nil)
}

func (c *Client) GetUniversityQR(ctx context.Context, universityID, diplomaID string) (privateapi.QRResponse, error) {
	query := url.Values{}
	query.Set("universityId", universityID)

	var response registryQRResponse
	if err := c.doJSON(ctx, http.MethodGet, "/internal/gateway/university/diplomas/"+diplomaID+"/qr?"+query.Encode(), nil, "", &response); err != nil {
		return privateapi.QRResponse{}, err
	}
	return privateapi.QRResponse{
		VerificationToken: response.VerificationToken,
		QRURL:             response.QRURL,
		ExpiresAt:         response.ExpiresAt,
	}, nil
}

func (c *Client) GetStudentDiploma(ctx context.Context, diplomaID string) (privateapi.StudentDiploma, error) {
	query := url.Values{}
	query.Set("diplomaId", diplomaID)

	var response registryDiplomaResponse
	if err := c.doJSON(ctx, http.MethodGet, "/internal/gateway/student/diploma?"+query.Encode(), nil, "", &response); err != nil {
		return privateapi.StudentDiploma{}, err
	}
	return mapStudentDiploma(response), nil
}

func (c *Client) CreateStudentShareLink(ctx context.Context, diplomaID string, ttlSeconds int) (privateapi.ShareLink, error) {
	query := url.Values{}
	query.Set("diplomaId", diplomaID)

	payload, err := json.Marshal(map[string]int{"ttlSeconds": ttlSeconds})
	if err != nil {
		return privateapi.ShareLink{}, fmt.Errorf("marshal share link payload: %w", err)
	}

	var response registryShareLinkResponse
	if err := c.doJSON(ctx, http.MethodPost, "/internal/gateway/student/share-link?"+query.Encode(), bytes.NewReader(payload), "application/json", &response); err != nil {
		return privateapi.ShareLink{}, err
	}

	return privateapi.ShareLink{
		ShareToken: response.ShareToken,
		ShareURL:   response.ShareURL,
		ExpiresAt:  response.ExpiresAt,
		TTLSeconds: ttlSeconds,
	}, nil
}

func (c *Client) DeleteStudentShareLink(ctx context.Context, diplomaID, token string) error {
	query := url.Values{}
	query.Set("diplomaId", diplomaID)
	return c.doJSON(ctx, http.MethodDelete, "/internal/gateway/student/share-link/"+path.Clean(token)+"?"+query.Encode(), nil, "", nil)
}

func (c *Client) doJSON(ctx context.Context, method, pathWithQuery string, body *bytes.Reader, contentType string, dest any) error {
	var requestBody io.Reader
	requestBody = body
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+pathWithQuery, requestBody)
	if err != nil {
		return fmt.Errorf("build registry request: %w", err)
	}
	if c.serviceToken != "" {
		request.Header.Set("Authorization", "Bearer "+c.serviceToken)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	request.Header.Set("Accept", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call registry internal api: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= 400 {
		var payload struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(response.Body).Decode(&payload)
		message := strings.TrimSpace(payload.Error)
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		return privateapi.NewAPIError(response.StatusCode, message)
	}

	if dest == nil {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode registry response: %w", err)
	}
	return nil
}

func mapUniversityDiploma(response registryDiplomaResponse) privateapi.UniversityDiplomaItem {
	return privateapi.UniversityDiplomaItem{
		ID:                response.ID,
		DiplomaNumber:     response.DiplomaNumber,
		UniversityCode:    response.UniversityCode,
		OwnerName:         response.OwnerName,
		OwnerNameMask:     response.OwnerNameMask,
		Program:           response.ProgramName,
		GraduationYear:    response.GraduationYear,
		Status:            mapStatus(response.Status),
		Hash:              response.RecordHash,
		VerificationToken: response.VerificationToken,
		CreatedAt:         response.CreatedAt,
		RevokedAt:         response.RevokedAt,
		RevokeReason:      response.RevokeReason,
	}
}

func mapStudentDiploma(response registryDiplomaResponse) privateapi.StudentDiploma {
	return privateapi.StudentDiploma{
		ID:                response.ID,
		DiplomaNumber:     response.DiplomaNumber,
		UniversityCode:    response.UniversityCode,
		Program:           response.ProgramName,
		GraduationYear:    response.GraduationYear,
		Status:            mapStatus(response.Status),
		VerificationToken: response.VerificationToken,
	}
}

func mapStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "valid":
		return "active"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

type registryDiplomaListResponse struct {
	Items []registryDiplomaResponse `json:"items"`
	Total int64                     `json:"total"`
}

type registryDiplomaResponse struct {
	ID                string     `json:"id"`
	UniversityCode    string     `json:"universityCode"`
	OwnerName         string     `json:"ownerName"`
	OwnerNameMask     string     `json:"ownerNameMask"`
	DiplomaNumber     string     `json:"diplomaNumber"`
	ProgramName       string     `json:"programName"`
	GraduationYear    *int       `json:"graduationYear"`
	RecordHash        string     `json:"recordHash"`
	Status            string     `json:"status"`
	VerificationToken string     `json:"verificationToken"`
	RevokedAt         *time.Time `json:"revokedAt"`
	RevokeReason      string     `json:"revokeReason"`
	CreatedAt         time.Time  `json:"createdAt"`
}

type registryImportAcceptedResponse struct {
	JobID  string `json:"jobId"`
	Status string `json:"status"`
}

type registryImportJobResponse struct {
	ID            string    `json:"id"`
	Status        string    `json:"status"`
	TotalRows     *int      `json:"totalRows"`
	ProcessedRows int       `json:"processedRows"`
	FailedRows    int       `json:"failedRows"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type registryImportErrorsResponse struct {
	Errors []registryImportError `json:"errors"`
}

type registryImportError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

type registryQRResponse struct {
	VerificationToken string    `json:"verificationToken"`
	QRURL             string    `json:"qrUrl"`
	ExpiresAt         time.Time `json:"expiresAt"`
}

type registryShareLinkResponse struct {
	ShareToken string    `json:"shareToken"`
	ShareURL   string    `json:"shareUrl"`
	ExpiresAt  time.Time `json:"expiresAt"`
}
