//go:build tools

// Package tools 不参与编译，只用于把依赖版本钉住（Go 社区标准的 tools.go 用法）。
package tools

import (
	// 空导入：把 google.golang.org/genproto 父模块钉在拆分之后的版本上。
	//
	// 该模块在 2023 年被拆成 googleapis/rpc、googleapis/api 等子模块，但老版本
	// (2019/2020) 里仍然带着 googleapis/rpc/status。本库的间接依赖
	// (prometheus/client_golang v1.13.0 → x/oauth2 → cloud.google.com/go v0.65.0)
	// 会把老版本拖进下游的模块图，下游在 go.work 工作区模式下合并依赖时就会撞
	// "ambiguous import: found package ... in multiple modules"。
	//
	// 没有任何代码真正用到这个模块，所以只在 go.mod 里写 require 会被 go mod tidy
	// 删掉；这样空导入钉住之后 tidy 就会保留它（并标成直接依赖）。
	_ "google.golang.org/genproto/googleapis/type/date"
)
