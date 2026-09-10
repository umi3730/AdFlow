package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	ad "github.com/umi3730/adflow/internal/agentassistant/domain"
	cd "github.com/umi3730/adflow/internal/campaign/domain"
	dd "github.com/umi3730/adflow/internal/decision/domain"
	ops "github.com/umi3730/adflow/internal/operations/application"
	rd "github.com/umi3730/adflow/internal/reporting/domain"
)

type ReportReader interface {
	Read(context.Context, rd.Filter) (rd.Report, error)
}
type TraceReader interface {
	Read(context.Context, string) (ops.RequestTrace, error)
}

// CampaignReader exposes only the campaign lookup needed by diagnosis.
type CampaignReader interface {
	FindByID(context.Context, string) (*cd.Campaign, error)
}

// CreativeReader exposes only the creative listing needed by diagnosis.
type CreativeReader interface {
	ListCreativesByCampaign(context.Context, string) ([]*cd.Creative, error)
}

type DiagnosisRequest struct {
	rd.Filter
	Question  string `json:"question"`
	RequestID string `json:"requestId,omitempty"`
}
type DiagnosisService struct {
	reports   ReportReader
	campaigns CampaignReader
	creatives CreativeReader
	traces    TraceReader
	provider  ad.DiagnosticProvider
}

func NewDiagnosisService(reports ReportReader, campaigns CampaignReader, creatives CreativeReader, traces TraceReader, provider ad.DiagnosticProvider) *DiagnosisService {
	return &DiagnosisService{reports: reports, campaigns: campaigns, creatives: creatives, traces: traces, provider: provider}
}
func (s *DiagnosisService) Diagnose(ctx context.Context, input DiagnosisRequest) (ad.Diagnosis, error) {
	if len([]rune(input.Question)) > 1000 || len([]rune(input.RequestID)) > 128 {
		return ad.Diagnosis{}, ad.ErrInvalidDiagnosis
	}
	ctx, cancel := context.WithTimeout(ctx, 9*time.Second)
	defer cancel()
	report, err := s.reports.Read(ctx, input.Filter)
	if err != nil {
		return ad.Diagnosis{}, err
	}
	c := ad.DiagnosisContext{Question: strings.TrimSpace(input.Question), Evidence: []ad.Evidence{}}
	add := func(id, title, detail, suggestion string) {
		c.Evidence = append(c.Evidence, ad.Evidence{ID: id, Title: title, Detail: detail, Suggestion: suggestion})
	}
	m := report.Summary
	add("report", "投放效果", fmt.Sprintf("北京时间 %s 至 %s，曝光 %d，点击 %d，转化 %d，已确认消耗 %.2f 元，转化价值 %.2f 元，CTR %.2f%%，CVR %.2f%%。", input.From.In(rd.Beijing).Format("01-02 15:04"), input.To.In(rd.Beijing).Format("01-02 15:04"), m.Impressions, m.Clicks, m.Conversions, float64(m.SpendFen)/100, float64(m.ValueFen)/100, m.CTR*100, m.CVR*100), "按相同时间范围比较计划或素材效果，每次只调整一个变量，再观察曝光、点击与转化变化。")
	if m.Impressions == 0 {
		add("no_impressions", "尚无曝光数据", "所选范围没有已计入统计的曝光。", "检查计划状态、投放时间、启用素材和测试流量；可填写请求 ID 进一步定位未投放原因。")
	}
	if m.Impressions > 0 && m.Clicks == 0 {
		add("no_clicks", "已有曝光但没有点击", fmt.Sprintf("%d 次曝光，0 次点击。", m.Impressions), "检查点击回传是否接通，并比较素材文案、图片和落地页；先补足样本再评估效果。")
	}
	if m.Clicks > 0 && m.Conversions == 0 {
		add("no_conversions", "点击尚未产生转化", fmt.Sprintf("%d 次点击，0 次转化。", m.Clicks), "检查转化回传与归因请求，确认落地页流程可完成，再对比不同人群的转化情况。")
	}
	if m.UnpricedImpressions > 0 {
		add("unpriced", "部分曝光缺少消耗凭据", fmt.Sprintf("%d 次曝光缺少可读取的已确认结算快照。", m.UnpricedImpressions), "在事件处理页核对结算记录，再使用完整消耗评估投放成本。")
	}
	if input.CampaignID != "" {
		campaign, err := s.campaigns.FindByID(ctx, input.CampaignID)
		if err != nil {
			return ad.Diagnosis{}, err
		}
		creatives, err := s.creatives.ListCreativesByCampaign(ctx, input.CampaignID)
		if err != nil {
			return ad.Diagnosis{}, err
		}
		active := 0
		for _, creative := range creatives {
			if creative.Status() == cd.CreativeActive {
				active++
			}
		}
		now := time.Now()
		period := campaign.Period()
		add("campaign", "当前计划配置", fmt.Sprintf("状态 %s，启用素材 %d 个。", campaign.Status(), active), "结合当前规则与所选时段的效果检查配置；需要调整时在广告计划页确认发布。")
		if version := campaign.ActiveVersion(); version != nil {
			rule := version.Targeting()
			price := version.ImpressionCost().Amount()
			if auction := version.Auction(); auction != nil {
				price = auction.BidFen
			}
			add("rules", "预算与投放规则", fmt.Sprintf("当前版本 v%d，日预算 %.2f 元，单次曝光出价 %.2f 元，每人每日频控 %d 次；ALL/ANY/NONE 条件数分别为 %d/%d/%d。", version.Number(), float64(version.DailyBudget().Amount())/100, float64(price)/100, version.FrequencyLimit(), len(rule.All), len(rule.Any), len(rule.None)), "结合目标人群检查条件是否过窄，检查预算和频控是否符合投放目标；先进行小流量测试再确认调整。")
		}
		if campaign.Status() != cd.StatusActive {
			add("inactive", "计划未启用", "当前计划不是投放中状态。", "草稿计划先确认规则并发布；已暂停计划确认投放周期后恢复。")
		}
		if now.Before(period.Start()) {
			add("not_started", "尚未到投放时间", "当前时间早于计划开始时间。", "等待投放开始，或在广告计划页调整计划时间。")
		}
		if !now.Before(period.End()) {
			add("ended", "投放周期已结束", "当前时间已达到或超过结束时间。", "创建新的投放周期并确认计划配置。")
		}
		if active == 0 {
			add("no_creative", "没有启用素材", "计划下启用素材数量为 0。", "在素材管理中添加或启用至少一个素材后再测试投放。")
		}
	}
	if input.RequestID != "" {
		trace, err := s.traces.Read(ctx, strings.TrimSpace(input.RequestID))
		if err != nil {
			return ad.Diagnosis{}, err
		}
		if input.CampaignID != "" && trace.Decision != nil && trace.Decision.Matched && trace.Decision.CampaignID != input.CampaignID {
			return ad.Diagnosis{}, ad.ErrInvalidDiagnosis
		}
		if trace.Decision != nil {
			reason := string(trace.Decision.Reason)
			title, action := reasonAdvice(trace.Decision.Reason)
			add("request", title, "指定请求的已保存决策结果："+reason, action)
		} else {
			add("request_pending", "尚无完整决策结果", "指定请求存在处理记录，但未保存完整决策。", "查看请求追踪中的执行状态，等待处理完成或排查依赖异常。")
		}
		if trace.Settlement != nil && trace.Settlement.Status != "SETTLED" {
			add("settlement", "曝光结算尚未完成", "结算状态："+trace.Settlement.Status, "在事件处理页查看等待结算或待核对记录，结算完成后再检查统计。")
		}
	}
	result, err := s.provider.Diagnose(ctx, c)
	if err != nil {
		return ad.Diagnosis{}, err
	}
	if err = ad.ValidateDiagnosis(result, c); err != nil {
		return ad.Diagnosis{}, err
	}
	result.Evidence = c.Evidence
	result.GeneratedAt = time.Now().UTC()
	return result, nil
}
func reasonAdvice(reason dd.Reason) (string, string) {
	switch reason {
	case dd.ReasonProfileNotFound:
		return "请求未找到画像", "先保存该用户画像，或在投放测试中选择已有用户。"
	case dd.ReasonNoCandidate:
		return "请求没有有效候选", "检查广告位是否一致、计划是否启用及投放时间是否有效。"
	case dd.ReasonNoCreative:
		return "请求因缺少素材未投放", "检查该请求对应计划的素材是否已添加并启用；补齐后使用新的请求 ID 再次测试，历史结果保持不变。"
	case dd.ReasonTargetingMiss:
		return "请求未匹配定向", "使用单次投放测试的规则检查，确认用户字段和标签；按业务目标调整定向条件。"
	case dd.ReasonFrequencyCapped:
		return "请求达到频控上限", "检查该用户的当日曝光次数，使用其他用户验证；根据投放目标评估频控设置。"
	case dd.ReasonBudgetExhausted:
		return "请求预算不足", "检查日预算、已消耗和预占金额，确认预算后再调整计划。"
	case dd.ReasonDependencyUnavailable:
		return "请求依赖暂不可用", "检查 MySQL、Redis 的健康状态和处理日志，恢复后重新测试。"
	case dd.ReasonMatched:
		return "请求已成功投放", "检查曝光、点击和转化是否回传，结合结算状态确认效果统计。"
	default:
		return "请求处理结果", "查看请求追踪，结合计划和事件状态进一步排查。"
	}
}
