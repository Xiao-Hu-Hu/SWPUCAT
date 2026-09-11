package repository

import (
	"SWPUCAT/internal/domain/report"
	"SWPUCAT/internal/infrastructure/database"
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ReportRepo struct {
	db *gorm.DB
}

func NewReportRepo(db *gorm.DB) *ReportRepo { return &ReportRepo{db: db} }

func (r *ReportRepo) CreatePeriod(ctx context.Context, period *report.ReportPeriod) error {
	model := &database.ReportPeriodModel{
		ID: period.ID, PeriodKey: period.PeriodKey, StartsOn: period.StartsOn, EndsOn: period.EndsOn,
		Timezone: period.Timezone, DueAt: period.DueAt, Status: string(period.Status), RosterVersion: period.RosterVersion,
		CreatedBy: period.CreatedBy, CreatedByName: period.CreatedByName, CreatedAt: period.CreatedAt, UpdatedAt: period.UpdatedAt,
	}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	period.ID, period.CreatedAt, period.UpdatedAt = model.ID, model.CreatedAt, model.UpdatedAt
	return nil
}

func (r *ReportRepo) FindPeriodByID(ctx context.Context, id int64) (*report.ReportPeriod, error) {
	var model database.ReportPeriodModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	period := toDomainReportPeriod(&model)
	var members []database.ReportPeriodMemberModel
	if err := r.db.WithContext(ctx).Where("period_id = ?", id).Order("user_id").Find(&members).Error; err != nil {
		return nil, err
	}
	period.Members = make([]report.PeriodMember, 0, len(members))
	for i := range members {
		period.Members = append(period.Members, toDomainPeriodMember(&members[i]))
	}
	return period, nil
}

func (r *ReportRepo) ListPeriods(ctx context.Context) ([]*report.ReportPeriod, error) {
	var models []database.ReportPeriodModel
	if err := r.db.WithContext(ctx).Order("starts_on DESC, id DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	periods := make([]*report.ReportPeriod, 0, len(models))
	for i := range models {
		period := toDomainReportPeriod(&models[i])
		var count int64
		if err := r.db.WithContext(ctx).Model(&database.ReportPeriodMemberModel{}).Where("period_id = ?", period.ID).Count(&count).Error; err != nil {
			return nil, err
		}
		period.Members = make([]report.PeriodMember, 0, count)
		for j := int64(0); j < count; j++ {
			period.Members = append(period.Members, report.PeriodMember{})
		}
		periods = append(periods, period)
	}
	return periods, nil
}

func (r *ReportRepo) AddPeriodMember(ctx context.Context, member *report.PeriodMember) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var period database.ReportPeriodModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&period, member.PeriodID).Error; err != nil {
			return err
		}
		if period.Status != string(report.PeriodStatusOpen) {
			return report.ErrInvalidPeriod
		}
		model := &database.ReportPeriodMemberModel{
			PeriodID: member.PeriodID, UserID: member.UserID, DisplayName: member.DisplayName,
			Required: member.Required, Exempt: member.Exempt, ExemptionReason: member.ExemptionReason,
			EffectiveFrom: member.EffectiveFrom, EffectiveUntil: member.EffectiveUntil,
		}
		if err := tx.Create(model).Error; err != nil {
			return err
		}
		return tx.Model(&database.ReportPeriodModel{}).Where("id = ?", member.PeriodID).Update("roster_version", gorm.Expr("roster_version + 1")).Error
	})
}

func (r *ReportRepo) CreateReport(ctx context.Context, item *report.Report) error {
	model := &database.ReportModel{
		ID: item.ID, PeriodID: item.PeriodID, ReportType: string(item.Type), AuthorID: item.AuthorID,
		AuthorName: item.AuthorName, Status: string(item.Status), Visibility: item.Visibility,
		CurrentVersion: item.CurrentVersion, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	item.ID, item.CreatedAt, item.UpdatedAt = model.ID, model.CreatedAt, model.UpdatedAt
	return nil
}

func (r *ReportRepo) FindReportByID(ctx context.Context, id int64) (*report.Report, error) {
	var model database.ReportModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, err
	}
	return toDomainReport(&model), nil
}

func (r *ReportRepo) ListReports(ctx context.Context, authorID int64, manager bool, periodID *int64, status report.ReportStatus, page, pageSize int) ([]*report.Report, error) {
	query := r.db.WithContext(ctx).Model(&database.ReportModel{})
	if !manager {
		query = query.Where("author_id = ?", authorID)
	}
	if periodID != nil {
		query = query.Where("period_id = ?", *periodID)
	}
	if status.IsValid() {
		query = query.Where("status = ?", string(status))
	}
	if page > 0 && pageSize > 0 {
		query = query.Offset((page - 1) * pageSize).Limit(pageSize)
	}
	var models []database.ReportModel
	if err := query.Order("updated_at DESC, id DESC").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]*report.Report, 0, len(models))
	for i := range models {
		items = append(items, toDomainReport(&models[i]))
	}
	return items, nil
}

func (r *ReportRepo) CreateVersion(ctx context.Context, version *report.ReportVersion, expectedCurrent int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var parent database.ReportModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&parent, version.ReportID).Error; err != nil {
			return err
		}
		if parent.CurrentVersion != expectedCurrent || version.Version != expectedCurrent+1 || parent.Status == string(report.ReportStatusArchived) {
			return report.ErrInvalidVersion
		}
		model := &database.ReportVersionModel{
			ID: version.ID, ReportID: version.ReportID, Version: version.Version, Status: string(version.Status),
			Title: version.Title, BodyMarkdown: version.BodyMarkdown, ContentHash: version.ContentHash,
			ParseStatus: string(version.ParseStatus), ParseError: version.ParseError, ParserVersion: version.ParserVersion,
			ParsedJSON: version.ParsedJSON, ParsedAt: version.ParsedAt, CreatedBy: version.CreatedBy, CreatedAt: version.CreatedAt,
		}
		if err := tx.Create(model).Error; err != nil {
			return err
		}
		if err := tx.Model(&database.ReportModel{}).Where("id = ? AND current_version = ?", version.ReportID, expectedCurrent).Updates(map[string]interface{}{
			"current_version": version.Version, "updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
		}).Error; err != nil {
			return err
		}
		version.ID, version.CreatedAt = model.ID, model.CreatedAt
		return nil
	})
}

func (r *ReportRepo) FindVersion(ctx context.Context, reportID int64, version int) (*report.ReportVersion, error) {
	var model database.ReportVersionModel
	if err := r.db.WithContext(ctx).Where("report_id = ? AND version = ?", reportID, version).First(&model).Error; err != nil {
		return nil, err
	}
	result := toDomainReportVersion(&model)
	var assets []database.ReportVersionAssetModel
	if err := r.db.WithContext(ctx).Where("report_version_id = ?", model.ID).Order("knowledge_item_id").Find(&assets).Error; err != nil {
		return nil, err
	}
	result.Assets = make([]report.ReportAsset, 0, len(assets))
	for i := range assets {
		result.Assets = append(result.Assets, toDomainReportAsset(&assets[i]))
	}
	return result, nil
}

func (r *ReportRepo) UpdateVersion(ctx context.Context, version *report.ReportVersion) error {
	if version == nil {
		return report.ErrInvalidVersion
	}
	updates := map[string]interface{}{
		"status": string(version.Status), "title": version.Title, "body_markdown": version.BodyMarkdown,
		"content_hash": version.ContentHash, "parse_status": string(version.ParseStatus), "parse_error": version.ParseError,
		"parser_version": version.ParserVersion, "parsed_json": version.ParsedJSON, "parsed_at": version.ParsedAt,
		"quality_status": version.QualityStatus, "quality_json": version.QualityJSON, "quality_checked_at": version.QualityCheckedAt,
		"submitted_by": nil, "submitted_at": version.SubmittedAt,
	}
	if version.SubmittedBy > 0 {
		updates["submitted_by"] = version.SubmittedBy
	}
	result := r.db.WithContext(ctx).Model(&database.ReportVersionModel{}).Where("id = ?", version.ID).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *ReportRepo) UpdateVersionQuality(ctx context.Context, versionID int64, status, resultJSON string, checkedAt *time.Time) error {
	result := r.db.WithContext(ctx).Model(&database.ReportVersionModel{}).Where("id = ?", versionID).Updates(map[string]interface{}{
		"quality_status": status, "quality_json": resultJSON, "quality_checked_at": checkedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *ReportRepo) SaveQualityDecision(ctx context.Context, versionID int64, findingIndex int, decision, note string, decidedBy int64) error {
	if versionID <= 0 || findingIndex < 0 || decidedBy <= 0 || (decision != "accepted" && decision != "rejected") {
		return report.ErrInvalidVersion
	}
	model := &database.ReportQualityDecisionModel{ReportVersionID: versionID, FindingIndex: findingIndex, Decision: decision, Note: note, DecidedBy: decidedBy}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "report_version_id"}, {Name: "finding_index"}}, DoUpdates: clause.AssignmentColumns([]string{"decision", "note", "decided_by", "decided_at"})}).Create(model).Error
}

func (r *ReportRepo) UpdateReportStatus(ctx context.Context, reportID int64, status report.ReportStatus) error {
	if !status.IsValid() {
		return report.ErrInvalidReport
	}
	return r.db.WithContext(ctx).Model(&database.ReportModel{}).Where("id = ?", reportID).Update("status", string(status)).Error
}

func (r *ReportRepo) AttachAsset(ctx context.Context, asset *report.ReportAsset) error {
	if asset == nil || asset.ReportVersionID <= 0 || asset.KnowledgeItemID <= 0 || asset.AssetRole == "" {
		return report.ErrInvalidVersion
	}
	return r.db.WithContext(ctx).Create(&database.ReportVersionAssetModel{
		ReportVersionID: asset.ReportVersionID, KnowledgeItemID: asset.KnowledgeItemID,
		AssetRole: asset.AssetRole, Locator: asset.Locator, Note: asset.Note,
	}).Error
}

func toDomainReportPeriod(model *database.ReportPeriodModel) *report.ReportPeriod {
	return &report.ReportPeriod{ID: model.ID, PeriodKey: model.PeriodKey, StartsOn: model.StartsOn, EndsOn: model.EndsOn,
		Timezone: model.Timezone, DueAt: model.DueAt, Status: report.PeriodStatus(model.Status), RosterVersion: model.RosterVersion,
		CreatedBy: model.CreatedBy, CreatedByName: model.CreatedByName, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt,
		Members: make([]report.PeriodMember, 0)}
}

func toDomainPeriodMember(model *database.ReportPeriodMemberModel) report.PeriodMember {
	return report.PeriodMember{PeriodID: model.PeriodID, UserID: model.UserID, DisplayName: model.DisplayName, Required: model.Required,
		Exempt: model.Exempt, ExemptionReason: model.ExemptionReason, EffectiveFrom: model.EffectiveFrom, EffectiveUntil: model.EffectiveUntil}
}

func toDomainReport(model *database.ReportModel) *report.Report {
	return &report.Report{ID: model.ID, PeriodID: model.PeriodID, Type: report.ReportType(model.ReportType), AuthorID: model.AuthorID,
		AuthorName: model.AuthorName, Status: report.ReportStatus(model.Status), Visibility: model.Visibility,
		CurrentVersion: model.CurrentVersion, CreatedAt: model.CreatedAt, UpdatedAt: model.UpdatedAt}
}

func toDomainReportVersion(model *database.ReportVersionModel) *report.ReportVersion {
	var submittedBy int64
	if model.SubmittedBy != nil {
		submittedBy = *model.SubmittedBy
	}
	return &report.ReportVersion{ID: model.ID, ReportID: model.ReportID, Version: model.Version, Status: report.VersionStatus(model.Status),
		Title: model.Title, BodyMarkdown: model.BodyMarkdown, ContentHash: model.ContentHash, ParseStatus: report.ParseStatus(model.ParseStatus),
		ParseError: model.ParseError, ParserVersion: model.ParserVersion, ParsedJSON: model.ParsedJSON, ParsedAt: model.ParsedAt,
		QualityStatus: model.QualityStatus, QualityJSON: model.QualityJSON, QualityCheckedAt: model.QualityCheckedAt,
		CreatedBy: model.CreatedBy, SubmittedBy: submittedBy, SubmittedAt: model.SubmittedAt, CreatedAt: model.CreatedAt, Assets: make([]report.ReportAsset, 0)}
}

func toDomainReportAsset(model *database.ReportVersionAssetModel) report.ReportAsset {
	return report.ReportAsset{ReportVersionID: model.ReportVersionID, KnowledgeItemID: model.KnowledgeItemID, AssetRole: model.AssetRole, Locator: model.Locator, Note: model.Note}
}

var _ report.Repository = (*ReportRepo)(nil)
