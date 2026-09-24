/*
Package s3 提供 sqlitereplica.Handler 的 S3 协议实现（aws-sdk-go-v2）。

S3 协议是对象存储的事实标准：AWS S3、阿里云 OSS 的 S3 兼容模式、腾讯
COS、华为 OBS、MinIO 等都支持，通过 Endpoint 指向对应 S3 兼容端点即可
通吃各厂商。自建网关（MinIO 等）需开启 UsePathStyle。

远端布局：<prefix>/<generation>/snapshots/*.snapshot.lz4 与
<prefix>/<generation>/wal/*.wal.lz4（prefix 为空时直接在桶根下）。
*/
package s3

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/LeeZXin/zsf/logger"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	s3api "github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/LeeZXin/zsf/sqlitereplica"
)

// defaultRegion 未配置时的默认 region（可按部署区域用 Config.Region 覆盖）。
const defaultRegion = "cn-shenzhen"

// maxKeys S3 单次批量删除的对象数上限。
const maxKeys = 1000

var _ sqlitereplica.Handler = (*Handler)(nil)

/*
Config S3 handler 配置。
*/
type Config struct {
	// Bucket 桶名，必填。
	Bucket string
	// Region 默认 cn-shenzhen。S3 兼容端点通常对 region 要求宽松（MinIO
	// 一般用 us-east-1），按目标服务配置。
	Region string
	// Endpoint 自定义 S3 兼容端点（自动补 https://）。为空时走 AWS 官方
	// endpoint；接阿里云 OSS S3 兼容模式/腾讯 COS/华为 OBS/MinIO 时必填。
	Endpoint string
	// Prefix 远端根路径（如 "sqlite-backup/db1"），为空时直接使用桶根。
	Prefix string
	// AccessKeyID/AccessKeySecret 静态凭证，都为空时走默认凭证链
	//（AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY 环境变量、~/.aws/credentials）。
	AccessKeyID     string
	AccessKeySecret string
	// UsePathStyle 使用 path-style 寻址（https://endpoint/bucket/key）。
	// MinIO 等自建网关需要开启；云厂商一般用 virtual-host style。
	UsePathStyle bool
}

/*
Handler S3 协议存储 handler。
*/
type Handler struct {
	client   *s3api.Client
	uploader *transfermanager.Client
	bucket   string
	prefix   string // 规范化后的根路径，无首尾 "/"
}

/*
NewHandler 校验配置并构建 S3 客户端。参数错误直接 Fatal（fast fail，
与 sqlitereplica.Sync 一致）。
*/
func NewHandler(cfg Config) *Handler {
	if cfg.Bucket == "" {
		logger.Logger.Fatal().Msg("s3: bucket name is required")
	}

	region := cfg.Region
	if region == "" {
		region = defaultRegion
	}

	loadOpts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}
	if cfg.AccessKeyID != "" && cfg.AccessKeySecret != "" {
		loadOpts = append(loadOpts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.AccessKeySecret, ""),
		))
	}
	if cfg.Endpoint != "" {
		endpoint := cfg.Endpoint
		if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
			endpoint = "https://" + endpoint
		}
		loadOpts = append(loadOpts, config.WithBaseEndpoint(endpoint))
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background(), loadOpts...)
	if err != nil {
		logger.Logger.Fatal().Err(err).Msg("s3: cannot load aws config")
	}

	client := s3api.NewFromConfig(awsCfg, func(o *s3api.Options) {
		o.UsePathStyle = cfg.UsePathStyle
	})

	return &Handler{
		client:   client,
		uploader: transfermanager.New(client),
		bucket:   cfg.Bucket,
		prefix:   strings.Trim(cfg.Prefix, "/"),
	}
}

/*
key 返回 name 对应的对象 key。
*/
func (h *Handler) key(name string) string {
	if h.prefix == "" {
		return name
	}
	return h.prefix + "/" + name
}

/*
keyPrefix 返回 prefix 的列举前缀（带尾 "/"，prefix 为空时为空串）。
*/
func (h *Handler) keyPrefix() string {
	if h.prefix == "" {
		return ""
	}
	return h.prefix + "/"
}

/*
Output 将 r 内容上传为单个对象。走 transfermanager：小对象单次 Put，
大对象（超过 partSize）自动分片，流式读取不落本地缓冲。
*/
func (h *Handler) Output(ctx context.Context, name string, r io.Reader) error {
	_, err := h.uploader.UploadObject(ctx, &transfermanager.UploadObjectInput{
		Bucket: aws.String(h.bucket),
		Key:    aws.String(h.key(name)),
		Body:   r,
	})
	if err != nil {
		return fmt.Errorf("s3: upload %s: %w", name, err)
	}
	return nil
}

/*
Restore 把 generation 下所有对象下载到 dir，保持相对布局。generation 为空时
恢复最新一代：列举 prefix 下所有对象，解析出合法 generation 名取字典序最大。
*/
func (h *Handler) Restore(ctx context.Context, generation string, dir string) (string, error) {
	if generation == "" {
		gens, err := h.listGenerations(ctx)
		if err != nil {
			return "", err
		} else if len(gens) == 0 {
			return "", sqlitereplica.ErrNoGeneration
		}
		generation = gens[len(gens)-1] // 字典序最大 = 最新代
	}

	keys, err := h.listKeys(ctx, h.keyPrefix()+generation+"/")
	if err != nil {
		return "", err
	} else if len(keys) == 0 {
		return "", sqlitereplica.ErrNoGeneration
	}

	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		rel := strings.TrimPrefix(key, h.keyPrefix()+generation+"/")
		if rel == "" || rel == key {
			continue // 非该代对象（如空目录占位），跳过
		}

		out, err := h.client.GetObject(ctx, &s3api.GetObjectInput{
			Bucket: aws.String(h.bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			return "", fmt.Errorf("s3: get object %s: %w", key, err)
		}

		dst := filepath.Join(dir, generation, filepath.FromSlash(rel))
		if err := writeObject(out.Body, dst); err != nil {
			_ = out.Body.Close()
			return "", err
		} else if err := out.Body.Close(); err != nil {
			return "", err
		}
	}

	return generation, nil
}

/*
Remove 删除 generation 下所有对象，幂等。批量删除每批最多 1000 个。
*/
func (h *Handler) Remove(ctx context.Context, generation string) error {
	if !sqlitereplica.IsGenerationName(generation) {
		return fmt.Errorf("s3: invalid generation %q", generation)
	}

	keys, err := h.listKeys(ctx, h.keyPrefix()+generation+"/")
	if err != nil {
		return err
	}

	objects := make([]s3types.ObjectIdentifier, 0, len(keys))
	for _, key := range keys {
		objects = append(objects, s3types.ObjectIdentifier{Key: aws.String(key)})
	}

	for len(objects) > 0 {
		n := min(len(objects), maxKeys)
		batch := objects[:n]
		objects = objects[n:]

		out, err := h.client.DeleteObjects(ctx, &s3api.DeleteObjectsInput{
			Bucket: aws.String(h.bucket),
			Delete: &s3types.Delete{
				Objects: batch,
				Quiet:   aws.Bool(true),
			},
		})
		if err != nil {
			return fmt.Errorf("s3: delete batch of %d objects: %w", n, err)
		}
		if len(out.Errors) > 0 {
			return fmt.Errorf("s3: delete batch of %d objects: %d errors, first: %s/%s",
				n, len(out.Errors), aws.ToString(out.Errors[0].Key), aws.ToString(out.Errors[0].Code))
		}
		// Quiet=true 时成功删除的 key 不会出现在 Deleted 里，不能按 Deleted 条数核对。
	}
	return nil
}

/*
listGenerations 列举 prefix 下的所有合法 generation 名，升序返回。
*/
func (h *Handler) listGenerations(ctx context.Context) ([]string, error) {
	keys, err := h.listKeys(ctx, h.keyPrefix())
	if err != nil {
		return nil, err
	}

	set := make(map[string]struct{})
	for _, key := range keys {
		rel := strings.TrimPrefix(key, h.keyPrefix())
		seg := strings.SplitN(rel, "/", 2)[0]
		if sqlitereplica.IsGenerationName(seg) {
			set[seg] = struct{}{}
		}
	}

	gens := make([]string, 0, len(set))
	for gen := range set {
		gens = append(gens, gen)
	}
	sort.Strings(gens)
	return gens, nil
}

/*
listKeys 列举指定前缀下的所有对象 key（自动翻页）。
*/
func (h *Handler) listKeys(ctx context.Context, prefix string) ([]string, error) {
	paginator := s3api.NewListObjectsV2Paginator(h.client, &s3api.ListObjectsV2Input{
		Bucket: aws.String(h.bucket),
		Prefix: aws.String(prefix),
	})

	var keys []string
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("s3: list objects prefix %q: %w", prefix, err)
		}
		for _, obj := range page.Contents {
			if obj.Key != nil {
				keys = append(keys, *obj.Key)
			}
		}
	}
	return keys, nil
}

/*
writeObject 把对象内容写到本地文件。
*/
func writeObject(r io.Reader, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
