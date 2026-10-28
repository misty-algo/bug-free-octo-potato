package dto

// ServiceConfig 是 P22 客服页配置出参（契约 3.7.3 / 3.9.6，config_dictionaries 现值）。
// key 无记录时对应字段为空串（不 500）。
type ServiceConfig struct {
	Hotline     string // hotline 现值
	ServiceTime string // service_time 现值
	ServiceSpec string // service_spec 现值（3.9.6 服务规范 content）
	FAQJSON     string // FAQ JSON 数组文本；无记录/非法缺省 "[]"
}
