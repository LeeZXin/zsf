package index

import (
	"github.com/LeeZXin/zsf-utils/quit"
	"github.com/LeeZXin/zsf/property/static"
	"github.com/blevesearch/bleve"
	_ "github.com/blevesearch/bleve/index/store/goleveldb"
	"github.com/blevesearch/bleve/mapping"
	"github.com/blevesearch/bleve/search/query"
	"github.com/robfig/cron/v3"
	"time"
)

const (
	LogDefaultPath = "logs/bleve"
)

var (
	LogIndex bleve.Index
)

func init() {
	if static.GetBool("logger.bleve.enabled") {
		m := bleve.NewIndexMapping()
		m.DefaultMapping = newBleveMapping()
		var err error
		LogIndex, err = bleve.NewUsing(LogDefaultPath, m, "upside_down", "goleveldb", map[string]any{
			"create_if_missing": true,
			"error_if_exists":   true,
		})
		if err != nil {
			LogIndex, err = bleve.Open(LogDefaultPath)
			if err != nil {
				panic(err)
			}
		}
		quit.AddShutdownHook(func() {
			LogIndex.Close()
		})
		// 开启定时任务清除n天前的日志
		if static.GetBool("logger.bleve.clean.enabled") {
			cronExp := static.GetString("logger.bleve.clean.cron")
			// 默认凌晨清除二十天之前的数据
			if cronExp == "" {
				cronExp = "0 0 0 * * ?"
			}
			cronTab := cron.New(cron.WithSeconds())
			_, err = cronTab.AddFunc(cronExp, func() {
				days := static.GetInt("logger.bleve.clean.days")
				if days <= 0 {
					days = 20
				}
				endTime := time.Now().Add(time.Duration(days) * 24 * time.Hour).UnixMilli()
				CleanBleveLog(CleanBleveLogReq{
					EndTime: endTime,
				})
			})
			if err != nil {
				panic(err)
			}
			cronTab.Start()
			quit.AddShutdownHook(func() {
				cronTab.Stop()
			})
		}
	}
}

func newBleveMapping() *mapping.DocumentMapping {
	document := bleve.NewDocumentMapping()
	document.AddFieldMappingsAt("timestamp", bleve.NewNumericFieldMapping())
	document.AddFieldMappingsAt("level", bleve.NewTextFieldMapping())
	document.AddFieldMappingsAt("content", bleve.NewTextFieldMapping())
	document.AddFieldMappingsAt("application", bleve.NewTextFieldMapping())
	return document
}

type SearchBleveLogReq struct {
	Level       string `json:"level"`
	BeginTime   int64  `json:"beginTime"`
	EndTime     int64  `json:"endTime"`
	Content     string `json:"content"`
	Application string `json:"application"`
	From        int    `json:"from"`
	Size        int    `json:"size"`
}

type CleanBleveLogReq struct {
	BeginTime int64 `json:"beginTime"`
	EndTime   int64 `json:"endTime"`
}

func CleanBleveLog(req CleanBleveLogReq) {
	if LogIndex == nil {
		return
	}
	var beginTime, endTime *float64
	if req.BeginTime > 0 {
		i := float64(req.BeginTime)
		beginTime = &i
	}
	if req.EndTime > 0 {
		i := float64(req.EndTime)
		endTime = &i
	}
	if beginTime == nil && endTime == nil {
		return
	}
	numeric := bleve.NewNumericRangeQuery(beginTime, endTime)
	numeric.SetField("timestamp")
	searchRequest := bleve.NewSearchRequest(numeric)
	searchRequest.Size = 1
	for {
		searchResult, err := LogIndex.Search(searchRequest)
		if err != nil {
			return
		}
		if len(searchResult.Hits) == 0 {
			return
		}
		batch := LogIndex.NewBatch()
		for _, hit := range searchResult.Hits {
			batch.Delete(hit.ID)
		}
		err = LogIndex.Batch(batch)
		if err != nil {
			return
		}
	}
}

func SearchBleveLog(reqDTO SearchBleveLogReq) ([]map[string]any, time.Duration, error) {
	if LogIndex == nil {
		return []map[string]any{}, 0, nil
	}
	queries := make([]query.Query, 0)
	if reqDTO.Level != "" {
		level := bleve.NewTermQuery(reqDTO.Level)
		level.SetField("level")
		queries = append(queries, level)
	}
	if reqDTO.Content != "" {
		content := bleve.NewMatchQuery(reqDTO.Content)
		content.SetField("content")
		queries = append(queries, content)
	}
	if reqDTO.Application != "" {
		application := bleve.NewTermQuery(reqDTO.Application)
		application.SetField("application")
		queries = append(queries, application)
	}
	if reqDTO.BeginTime > 0 && reqDTO.EndTime > 0 {
		b, e := float64(reqDTO.BeginTime), float64(reqDTO.EndTime)
		numeric := bleve.NewNumericRangeQuery(&b, &e)
		numeric.SetField("timestamp")
		queries = append(queries, numeric)
	} else if reqDTO.BeginTime > 0 {
		b := float64(reqDTO.BeginTime)
		numeric := bleve.NewNumericRangeQuery(&b, nil)
		numeric.SetField("timestamp")
		queries = append(queries, numeric)
	} else if reqDTO.EndTime > 0 {
		e := float64(reqDTO.EndTime)
		numeric := bleve.NewNumericRangeQuery(nil, &e)
		numeric.SetField("timestamp")
		queries = append(queries, numeric)
	}
	searchRequest := bleve.NewSearchRequest(bleve.NewConjunctionQuery(queries...))
	searchRequest.From = reqDTO.From
	if reqDTO.Size <= 0 || reqDTO.Size > 500 {
		reqDTO.Size = 500
	}
	searchRequest.Size = reqDTO.Size
	searchRequest.Fields = []string{"*"}
	searchResult, err := LogIndex.Search(searchRequest)
	if err != nil {
		return nil, 0, err
	}
	ret := make([]map[string]any, 0, len(searchResult.Hits))
	for _, hit := range searchResult.Hits {
		ret = append(ret, hit.Fields)
	}
	return ret, searchResult.Took, nil
}
