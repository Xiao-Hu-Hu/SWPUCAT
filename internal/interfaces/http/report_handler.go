package http

import (
	reportapp "SWPUCAT/internal/application/report"
	domain "SWPUCAT/internal/domain/report"
	"SWPUCAT/internal/domain/shared"
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ReportHandler struct {
	service *reportapp.Service
}

func NewReportHandler(service *reportapp.Service) *ReportHandler {
	return &ReportHandler{service: service}
}

func (h *ReportHandler) CreatePeriod(c *gin.Context) {
	var req reportapp.CreatePeriodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid report period request")
		return
	}
	item, err := h.service.CreatePeriod(c.Request.Context(), GetUserID(c), GetUsername(c), req)
	if err != nil {
		handleReportError(c, err)
		return
	}
	Created(c, item)
}

func (h *ReportHandler) ListPeriods(c *gin.Context) {
	items, err := h.service.ListPeriods(c.Request.Context())
	if err != nil {
		InternalError(c, "failed to list report periods")
		return
	}
	Success(c, items)
}

func (h *ReportHandler) AddPeriodMember(c *gin.Context) {
	periodID, ok := reportIDParam(c, "period id")
	if !ok {
		return
	}
	var req reportapp.AddMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid report period member request")
		return
	}
	if err := h.service.AddPeriodMember(c.Request.Context(), periodID, req); err != nil {
		handleReportError(c, err)
		return
	}
	Created(c, nil)
}

func (h *ReportHandler) CreateReport(c *gin.Context) {
	var req reportapp.CreateReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid report request")
		return
	}
	if req.ReportType == "" {
		req.ReportType = domain.ReportTypeMonthly
	}
	if req.ReportType == domain.ReportTypeTeam && !IsCaptain(c) {
		Forbidden(c, "only captain or super admin can create team reports")
		return
	}
	item, err := h.service.CreateReport(c.Request.Context(), GetUserID(c), GetUsername(c), req)
	if err != nil {
		handleReportError(c, err)
		return
	}
	Created(c, item)
}

func (h *ReportHandler) ListReports(c *gin.Context) {
	var periodID *int64
	if raw := strings.TrimSpace(c.Query("period_id")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			BadRequest(c, "invalid period_id")
			return
		}
		periodID = &value
	}
	page, pageSize, ok := reportPagination(c)
	if !ok {
		return
	}
	items, err := h.service.ListReports(c.Request.Context(), GetUserID(c), IsCaptain(c), periodID, domain.ReportStatus(strings.TrimSpace(c.Query("status"))), page, pageSize)
	if err != nil {
		InternalError(c, "failed to list reports")
		return
	}
	Success(c, items)
}

func (h *ReportHandler) GetReport(c *gin.Context) {
	id, ok := reportIDParam(c, "report id")
	if !ok {
		return
	}
	item, err := h.service.GetReport(c.Request.Context(), id, GetUserID(c), IsCaptain(c))
	if err != nil {
		handleReportError(c, err)
		return
	}
	Success(c, item)
}

func (h *ReportHandler) CreateVersion(c *gin.Context) {
	reportID, ok := reportIDParam(c, "report id")
	if !ok {
		return
	}
	var req reportapp.CreateVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid report version request")
		return
	}
	item, err := h.service.CreateVersion(c.Request.Context(), reportID, GetUserID(c), IsCaptain(c), req)
	if err != nil {
		handleReportError(c, err)
		return
	}
	Created(c, item)
}

func (h *ReportHandler) GetVersion(c *gin.Context) {
	reportID, version, ok := reportVersionParams(c)
	if !ok {
		return
	}
	item, err := h.service.GetVersion(c.Request.Context(), reportID, version, GetUserID(c), IsCaptain(c))
	if err != nil {
		handleReportError(c, err)
		return
	}
	Success(c, item)
}

func (h *ReportHandler) SubmitVersion(c *gin.Context) {
	reportID, version, ok := reportVersionParams(c)
	if !ok {
		return
	}
	item, err := h.service.SubmitVersion(c.Request.Context(), reportID, version, GetUserID(c), IsCaptain(c))
	if err != nil {
		handleReportError(c, err)
		return
	}
	Success(c, item)
}

func (h *ReportHandler) ReviewVersion(c *gin.Context) {
	reportID, version, ok := reportVersionParams(c)
	if !ok {
		return
	}
	var req struct {
		Approved bool `json:"approved"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid review request")
		return
	}
	item, err := h.service.ReviewVersion(c.Request.Context(), reportID, version, req.Approved, IsCaptain(c))
	if err != nil {
		handleReportError(c, err)
		return
	}
	Success(c, item)
}

func (h *ReportHandler) AttachAsset(c *gin.Context) {
	reportID, version, ok := reportVersionParams(c)
	if !ok {
		return
	}
	var req reportapp.AttachAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "invalid report asset request")
		return
	}
	item, err := h.service.AttachAsset(c.Request.Context(), reportID, version, GetUserID(c), IsCaptain(c), req)
	if err != nil {
		handleReportError(c, err)
		return
	}
	Created(c, item)
}

func reportIDParam(c *gin.Context, label string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		BadRequest(c, "invalid "+label)
		return 0, false
	}
	return id, true
}

func reportVersionParams(c *gin.Context) (int64, int, bool) {
	reportID, ok := reportIDParam(c, "report id")
	if !ok {
		return 0, 0, false
	}
	version, err := strconv.Atoi(c.Param("version"))
	if err != nil || version <= 0 {
		BadRequest(c, "invalid report version")
		return 0, 0, false
	}
	return reportID, version, true
}

func reportPagination(c *gin.Context) (int, int, bool) {
	page, pageSize := 1, 20
	var err error
	if raw := c.Query("page"); raw != "" {
		page, err = strconv.Atoi(raw)
		if err != nil || page <= 0 {
			BadRequest(c, "invalid page")
			return 0, 0, false
		}
	}
	if raw := c.Query("page_size"); raw != "" {
		pageSize, err = strconv.Atoi(raw)
		if err != nil || pageSize <= 0 || pageSize > 100 {
			BadRequest(c, "page_size must be between 1 and 100")
			return 0, 0, false
		}
	}
	return page, pageSize, true
}

func handleReportError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, shared.ErrForbidden):
		Forbidden(c, "not allowed to access this report")
	case errors.Is(err, shared.ErrConflict), errors.Is(err, domain.ErrInvalidVersion), errors.Is(err, domain.ErrInvalidVersionState), errors.Is(err, domain.ErrDuplicateMember), errors.Is(err, domain.ErrDuplicateAsset):
		Conflict(c, "report state conflicts with this request")
	case errors.Is(err, shared.ErrValidation), errors.Is(err, domain.ErrInvalidPeriod), errors.Is(err, domain.ErrInvalidReport):
		BadRequest(c, "invalid report data")
	case errors.Is(err, gorm.ErrRecordNotFound):
		NotFound(c, "report not found")
	default:
		InternalError(c, "report operation failed")
	}
}
