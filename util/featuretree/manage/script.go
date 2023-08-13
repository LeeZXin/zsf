package manage

import (
	"github.com/LeeZXin/zsf/util/luautil"
	"sync/atomic"
)

var (
	scriptFeatureConfigMap = atomic.Value{}
)

func init() {
	scriptFeatureConfigMap.Store(map[string]*luautil.CachedScript{})
}

func RefreshScriptFeatureConfigMap(configMap map[string]*luautil.CachedScript) {
	if configMap == nil {
		return
	}
	scriptFeatureConfigMap.Store(configMap)
}

func GetScriptFeatureConfigMap() map[string]*luautil.CachedScript {
	return scriptFeatureConfigMap.Load().(map[string]*luautil.CachedScript)
}

func LoadScriptFeatureConfig(featureKey string) (*luautil.CachedScript, bool) {
	m := GetScriptFeatureConfigMap()
	val, ok := m[featureKey]
	return val, ok
}
