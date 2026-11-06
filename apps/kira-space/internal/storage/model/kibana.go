package model

// KibanaField mirrors packages/shared/domain/kibana.ts's kibanaFieldSchema.
type KibanaField struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"` // Kibana type: string, number, date, boolean, ip, geo_point, ...
	ESTypes      []string `json:"esTypes"`
	Searchable   bool     `json:"searchable"`
	Aggregatable bool     `json:"aggregatable"`
	Nested       *string  `json:"nested"`      // subType.nested.path
	MultiParent  *string  `json:"multiParent"` // subType.multi.parent
	Runtime      bool     `json:"runtime"`
}

// KibanaDataView is one Kibana data view. Fields is empty in a list result and sorted by name in a
// detail result.
type KibanaDataView struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Title         string        `json:"title"` // the index pattern
	TimeFieldName *string       `json:"timeFieldName"`
	Fields        []KibanaField `json:"fields"`
}

// KibanaTimeRange bounds a search: ISO-8601 or ES date math ("now-15m").
type KibanaTimeRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// KibanaFilter is one filter pill. Operator is one of is, is_not, is_one_of, is_not_one_of, exists,
// does_not_exist, between (between: Values is [from, to], either may be empty).
type KibanaFilter struct {
	Field    string   `json:"field"`
	Operator string   `json:"operator"`
	Values   []string `json:"values"`
	Disabled bool     `json:"disabled"`
}

// KibanaSort orders hits; Order is asc or desc.
type KibanaSort struct {
	Field string `json:"field"`
	Order string `json:"order"`
}

// KibanaDiscoverRequest is the statement text of an execute() on a kibana connection: one request
// per run. Decoded with DisallowUnknownFields, so a typo fails instead of vanishing.
type KibanaDiscoverRequest struct {
	DataView  string           `json:"dataView"`
	Query     string           `json:"query"`
	Language  string           `json:"language"` // kuery | lucene
	TimeRange *KibanaTimeRange `json:"timeRange"`
	Filters   []KibanaFilter   `json:"filters"`
	Sort      []KibanaSort     `json:"sort"`
	Size      int              `json:"size"`
	TimeZone  string           `json:"timeZone"` // IANA; "" = UTC
	// Columns selects fields: the page is a table of the time field plus these. Empty = documents.
	Columns []string `json:"columns,omitempty"`
}

// KibanaHistogramRequest asks for a hit-count date histogram of Request (Size and Sort ignored).
type KibanaHistogramRequest struct {
	ConnectionID string                `json:"connectionId"`
	OpID         string                `json:"opId"`
	TabID        string                `json:"tabId"`
	Request      KibanaDiscoverRequest `json:"request"`
	Interval     string                `json:"interval"` // auto | m | h | d | w | M | y
}

// KibanaHistogramBucket is one bar; Key is epoch milliseconds.
type KibanaHistogramBucket struct {
	Key   int64 `json:"key"`
	Count int64 `json:"count"`
}

// KibanaHistogram is the date histogram plus the total hit count.
type KibanaHistogram struct {
	Total    int64                   `json:"total"`
	Relation string                  `json:"relation"` // eq | gte
	Interval string                  `json:"interval"` // the effective interval, e.g. "30s", "1h", "1w"
	Buckets  []KibanaHistogramBucket `json:"buckets"`
}

// KibanaSuggestRequest asks for values of Field starting with Prefix. Request.Query is the query
// without the value under the cursor.
type KibanaSuggestRequest struct {
	ConnectionID string                `json:"connectionId"`
	OpID         string                `json:"opId"`
	TabID        string                `json:"tabId"`
	Request      KibanaDiscoverRequest `json:"request"`
	Field        string                `json:"field"`
	Prefix       string                `json:"prefix"`
}

// KibanaSuggestions lists values; Complete reports that every distinct value matching Prefix is in it.
type KibanaSuggestions struct {
	Values   []string `json:"values"`
	Complete bool     `json:"complete"`
}

// KibanaDataViewsRequest and KibanaDataViewRequest are the bridge arguments of the data view reads.
type KibanaDataViewsRequest struct {
	ConnectionID string `json:"connectionId"`
	OpID         string `json:"opId"`
}

type KibanaDataViewRequest struct {
	ConnectionID string `json:"connectionId"`
	OpID         string `json:"opId"`
	ID           string `json:"id"`
}
