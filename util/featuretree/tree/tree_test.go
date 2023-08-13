package tree

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/LeeZXin/zsf/util/featuretree/manage"
	"github.com/LeeZXin/zsf/util/luautil"
	lua "github.com/yuin/gopher-lua"
	"testing"
	"time"
)

func TestCreateTree(t *testing.T) {
	manage.RefreshScriptFeatureConfigMap(map[string]*luautil.CachedScript{
		"user_mock_script": luautil.NewCachedScript("return fn(\"aaaa\")"),
	})
	e, _ := luautil.NewScriptExecutor(500, 10, map[string]lua.LGFunction{
		"fn": func(state *lua.LState) int {
			args := luautil.GetFnArgs(state)
			state.Push(args[0])
			return 1
		},
	})
	RegisterDefaultScriptExecutor(e)
	str := `{
				"or": [
					{
						"featureType": "script",
						"featureKey": "user_mock_script",
						"featureName": "脚本测试",      
						"dataType": "string",
						"operator": "eq",      
						"value": "aaaas"
					},
					{
						"featureType": "message",
						"featureKey": "user_purchase_product_code",
						"featureName": "用户成交产品",      
						"dataType": "script",
						"operator": "script",      
						"value": "params.userValue == 'ff5hh555d'"
					},
					{
						"featureType": "message",
						"featureKey": "user_purchase_product_code",
						"featureName": "用户成交产品",      
						"dataType": "string",
						"operator": "regMatch",      
						"value": "^s\\d+$"
					},
					{
						"featureType": "message",
						"featureKey": "user_purchase_product_codeddd",
						"featureName": "用户成交产品",      
						"dataType": "number",
						"operator": "between",      
						"value": "1234,124325"
					}
				]
			}`
	var treePlainInfo PlainInfo
	err := json.Unmarshal([]byte(str), &treePlainInfo)
	if err != nil {
		panic(err)
	}
	tree, err := BuildFeatureTree("xx", &treePlainInfo)
	if err != nil {
		panic(err)
	}
	m := map[string]any{
		"user_purchase_product_code":    "ff5hh555",
		"user_purchase_product_codeddd": 123000004,
	}
	timeout, cancelFunc := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancelFunc()
	for i := 0; i < 10; i++ {
		treeAnalyser := InitTreeAnalyser(BuildFeatureAnalyseContext(tree, m, timeout))
		analyseResult := treeAnalyser.Analyse()
		fmt.Println(analyseResult.GetMissResultDetailDesc())
	}

}

func BenchmarkCreateTree(t *testing.B) {
	manage.RefreshScriptFeatureConfigMap(map[string]*luautil.CachedScript{
		"user_mock_script": luautil.NewCachedScript(`return 'hello world'`),
	})
	str := `{
				"or": [
					{
						"featureType": "script",
						"featureKey": "user_mock_script",
						"featureName": "脚本测试",      
						"dataType": "string",
						"operator": "eq",      
						"value": "hello"
					},
					{
						"featureType": "message",
						"featureKey": "user_purchase_product_code",
						"featureName": "用户成交产品",      
						"dataType": "script",
						"operator": "script",      
						"value": "params.userValue"
					},
					{
						"featureType": "message",
						"featureKey": "user_purchase_product_code",
						"featureName": "用户成交产品",      
						"dataType": "string",
						"operator": "regMatch",      
						"value": "^s\\d+$"
					},
					{
						"featureType": "message",
						"featureKey": "user_purchase_product_codeddd",
						"featureName": "用户成交产品",      
						"dataType": "number",
						"operator": "between",      
						"value": "1234,124325"
					}
				]
			}`
	var treePlainInfo PlainInfo
	err := json.Unmarshal([]byte(str), &treePlainInfo)
	if err != nil {
		panic(err)
	}
	tree, err := BuildFeatureTree("xx", &treePlainInfo)
	if err != nil {
		panic(err)
	}
	m := map[string]any{
		"user_purchase_product_code":    "ff5hh555",
		"user_purchase_product_codeddd": 1,
	}
	for i := 0; i < t.N; i++ {
		treeAnalyser := InitTreeAnalyser(BuildFeatureAnalyseContext(tree, m, context.Background()))
		_ = treeAnalyser.Analyse()
	}

}
