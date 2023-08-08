package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"hash/crc32"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 默认分64segment
const (
	segmentSize = 64
)

// ServiceInfo 注册服务信息
type ServiceInfo struct {
	ServiceName   string        `json:"serviceName,omitempty"`
	Ip            string        `json:"ip,omitempty"`
	Port          int           `json:"port,omitempty"`
	InstanceId    string        `json:"instanceId,omitempty"`
	Weight        int           `json:"weight,omitempty"`
	Version       string        `json:"version,omitempty"`
	LeaseDuration time.Duration `json:"leaseDuration,omitempty"`
	ExpireTime    time.Time     `json:"expireTime"`
}

type RegistryServer struct {
	serverPort int
	segments   []*segment
	httpServer *http.Server
	httpEngine *gin.Engine

	startOnce sync.Once
	stopOnce  sync.Once

	ctx      context.Context
	cancelFn context.CancelFunc

	token  string
	logger Logger
}

// NewRegistryServer 利用gin生成一个服务端
func NewRegistryServer(serverPort int, token string, logger Logger) *RegistryServer {
	if logger == nil {
		logger = &DiscardLogger{}
	}
	segments := make([]*segment, 0, segmentSize)
	for i := 0; i < segmentSize; i++ {
		segments = append(segments, &segment{
			serviceCache: make(map[string]map[string]*ServiceInfo, 8),
			cacheMu:      sync.Mutex{},
		})
	}
	gin.SetMode(gin.ReleaseMode)
	httpEngine := gin.New()
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", serverPort),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  5 * time.Minute,
		Handler:      httpEngine,
	}
	ctx, cancelFunc := context.WithCancel(context.Background())
	ret := &RegistryServer{
		serverPort: serverPort,
		segments:   segments,
		httpServer: server,
		httpEngine: httpEngine,
		ctx:        ctx,
		cancelFn:   cancelFunc,
		token:      token,
		logger:     logger,
	}
	ret.HttpRouter(httpEngine)
	return ret
}

func (r *RegistryServer) Start(startHttpServer bool) {
	r.startOnce.Do(func() {
		// 启动http服务
		if startHttpServer {
			go func() {
				r.logger.Info("start http server port:" + strconv.Itoa(r.serverPort))
				err := r.httpServer.ListenAndServe()
				if err != nil {
					r.logger.Errorf("start http server fail: %s", err.Error())
					panic(err)
				}
			}()
		}
		// 清理服务过期数据
		go func() {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					r.ClearExpire()
					break
				case <-r.ctx.Done():
					return
				}
			}
		}()
	})
}

func (r *RegistryServer) HttpRouter(engine *gin.Engine) {
	group := engine.Group("/registry")
	{
		// 注册服务
		group.POST("/registerService", r.gin_RegisterService)
		// 注销服务
		group.POST("/deregisterService", r.gin_DeregisterService)
		// 续期
		group.POST("/passTTL", r.gin_PassTTL)
		// 获取服务列表
		group.POST("/getServiceInfoList", r.gin_GetServiceInfoList)
	}
}

func (r *RegistryServer) Stop() {
	r.stopOnce.Do(func() {
		r.logger.Info("register server stop")
		r.httpServer.Shutdown(context.Background())
		r.cancelFn()
	})
}

// gin_RegisterService 注册service
func (r *RegistryServer) gin_RegisterService(c *gin.Context) {
	if r.checkToken(c) {
		var reqDTO RegisterServiceReqDTO
		err := c.ShouldBind(&reqDTO)
		if err != nil {
			c.JSON(http.StatusBadRequest, DefaultFailBindArgResp)
			return
		}
		reqStr, _ := json.Marshal(reqDTO)
		r.logger.Infof("register service: %s", reqStr)
		r.Register(reqDTO)
		c.JSON(http.StatusOK, DefaultSuccessResp)
	}
}

// gin_PassTTL 续期
func (r *RegistryServer) gin_PassTTL(c *gin.Context) {
	if r.checkToken(c) {
		var reqDTO PassTtlReqDTO
		err := c.ShouldBind(&reqDTO)
		if err != nil {
			c.JSON(http.StatusBadRequest, DefaultFailBindArgResp)
			return
		}
		reqStr, _ := json.Marshal(reqDTO)
		r.logger.Infof("pass ttl: %s", reqStr)
		err = r.PassTTL(reqDTO)
		if err != nil {
			c.JSON(http.StatusOK, BaseResp{
				Code:    ExecuteFailCode,
				Message: err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, DefaultSuccessResp)
	}
}

// gin_DeregisterService 注销
func (r *RegistryServer) gin_DeregisterService(c *gin.Context) {
	if r.checkToken(c) {
		var reqDTO DeregisterServiceReqDTO
		err := c.ShouldBind(&reqDTO)
		if err != nil {
			c.JSON(http.StatusBadRequest, DefaultFailBindArgResp)
			return
		}
		reqStr, _ := json.Marshal(reqDTO)
		r.logger.Infof("deregister service: %s", reqStr)
		err = r.Deregister(reqDTO)
		if err != nil {
			c.JSON(http.StatusOK, BaseResp{
				Code:    ExecuteFailCode,
				Message: err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, DefaultSuccessResp)
	}
}

// gin_GetServiceInfoList 获取服务列表
func (r *RegistryServer) gin_GetServiceInfoList(c *gin.Context) {
	if r.checkToken(c) {
		var reqDTO GetServiceInfoListReqDTO
		err := c.ShouldBind(&reqDTO)
		if err != nil {
			c.JSON(http.StatusBadRequest, DefaultFailBindArgResp)
			return
		}
		infoList := r.GetServiceInfoList(reqDTO.ServiceName)
		dtoList := make([]ServiceInfoDTO, 0, len(infoList))
		for _, info := range infoList {
			dtoList = append(dtoList, ServiceInfoDTO{
				ServiceName:   info.ServiceName,
				Ip:            info.Ip,
				Port:          info.Port,
				InstanceId:    info.InstanceId,
				Weight:        info.Weight,
				Version:       info.Version,
				LeaseDuration: int(info.LeaseDuration.Seconds()),
			})
		}
		c.JSON(http.StatusOK, GetServiceInfoListRespDTO{
			BaseResp:    DefaultSuccessResp,
			ServiceList: dtoList,
		})
	}
}

// checkToken 检查token
func (r *RegistryServer) checkToken(c *gin.Context) bool {
	t := c.Request.Header.Get("z-token")
	if t != r.token {
		c.JSON(http.StatusUnauthorized, BaseResp{
			Code:    UnauthorizedCode,
			Message: "unauthorized",
		})
		return false
	}
	return true
}

// Register 注册service
func (r *RegistryServer) Register(reqDTO RegisterServiceReqDTO) {
	r.getSegment(reqDTO.ServiceName).Register(reqDTO)
}

// PassTTL 续期
func (r *RegistryServer) PassTTL(reqDTO PassTtlReqDTO) error {
	return r.getSegment(reqDTO.ServiceName).PassTTL(reqDTO)
}

// Deregister 注销
func (r *RegistryServer) Deregister(reqDTO DeregisterServiceReqDTO) error {
	return r.getSegment(reqDTO.ServiceName).Deregister(reqDTO)
}

// GetServiceInfoList 获取服务列表
func (r *RegistryServer) GetServiceInfoList(serviceName string) []ServiceInfo {
	return r.getSegment(serviceName).GetServiceInfoList(serviceName)
}

// ClearExpire 清理过期数据
func (r *RegistryServer) ClearExpire() {
	for _, seg := range r.segments {
		seg.ClearExpire()
	}
}

func (r *RegistryServer) getSegment(serviceName string) *segment {
	hashRet := crc32.ChecksumIEEE([]byte(serviceName))
	return r.segments[int(hashRet)&0x3f]
}

type segment struct {
	serviceCache map[string]map[string]*ServiceInfo
	cacheMu      sync.Mutex
}

func (r *segment) Register(reqDTO RegisterServiceReqDTO) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	duration := time.Duration(reqDTO.LeaseDuration) * time.Second
	info := &ServiceInfo{
		ServiceName:   reqDTO.ServiceName,
		Ip:            reqDTO.Ip,
		Port:          reqDTO.Port,
		InstanceId:    reqDTO.InstanceId,
		Weight:        reqDTO.Weight,
		Version:       reqDTO.Version,
		LeaseDuration: duration,
		ExpireTime:    time.Now().Add(duration),
	}
	infoMap, b := r.serviceCache[info.ServiceName]
	if b {
		infoMap[info.InstanceId] = info
	} else {
		infoMap = make(map[string]*ServiceInfo, 8)
		infoMap[info.InstanceId] = info
		r.serviceCache[info.ServiceName] = infoMap
	}
}

func (r *segment) PassTTL(reqDTO PassTtlReqDTO) error {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	infoMap, b := r.serviceCache[reqDTO.ServiceName]
	if !b {
		return errors.New("service name not found")
	}
	info, b := infoMap[reqDTO.InstanceId]
	if !b || info.ExpireTime.Before(time.Now()) {
		return errors.New("instanceId not found")
	}
	info.ExpireTime = time.Now().Add(info.LeaseDuration)
	return nil
}

func (r *segment) Deregister(reqDTO DeregisterServiceReqDTO) error {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	infoMap, b := r.serviceCache[reqDTO.ServiceName]
	if !b {
		return errors.New("service name not found")
	}
	_, b = infoMap[reqDTO.InstanceId]
	if !b {
		return errors.New("instanceId not found")
	}
	delete(infoMap, reqDTO.InstanceId)
	return nil
}

func (r *segment) GetServiceInfoList(serviceName string) []ServiceInfo {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	infoMap, b := r.serviceCache[serviceName]
	if !b {
		return []ServiceInfo{}
	}
	ret := make([]ServiceInfo, 0, len(infoMap))
	now := time.Now()
	for _, info := range infoMap {
		i := info
		if i.ExpireTime.Before(now) {
			continue
		}
		ret = append(ret, ServiceInfo{
			ServiceName:   i.ServiceName,
			Ip:            i.Ip,
			Port:          i.Port,
			InstanceId:    i.InstanceId,
			Weight:        i.Weight,
			Version:       i.Version,
			LeaseDuration: i.LeaseDuration,
			ExpireTime:    i.ExpireTime,
		})
	}
	sort.Slice(ret, func(i, j int) bool {
		return strings.Compare(ret[i].InstanceId, ret[j].InstanceId) < 0
	})
	return ret
}

func (r *segment) ClearExpire() {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	now := time.Now()
	for serviceName, infoMap := range r.serviceCache {
		for instanceId, info := range infoMap {
			if info.ExpireTime.Before(now) {
				delete(infoMap, instanceId)
			}
		}
		if len(infoMap) == 0 {
			delete(r.serviceCache, serviceName)
		}
	}
}
