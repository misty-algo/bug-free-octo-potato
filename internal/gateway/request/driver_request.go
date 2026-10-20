package request

// CreateDriverRequest 是 POST /api/v1/drivers 使用的 JSON 载荷。
type CreateDriverRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

// UpdateDriverRequest 是 PUT /api/v1/drivers/:id 使用的 JSON 载荷。
type UpdateDriverRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

// ListDriverRequest 是 GET /api/v1/drivers 使用的查询参数载荷。
type ListDriverRequest struct {
	Page     int32 `form:"page"`
	PageSize int32 `form:"page_size"`
}
