module github.com/e2b-dev/infra/packages/fc-provider

go 1.25.4

require (
	github.com/alibabacloud-go/darabonba-openapi/v2 v2.1.14
	github.com/alibabacloud-go/fc-20230330/v4 v4.7.1
	github.com/alibabacloud-go/tea v1.3.13
	github.com/alibabacloud-go/tea-utils/v2 v2.0.7
	github.com/e2b-dev/infra/packages/shared v0.0.0
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/alibabacloud-go/alibabacloud-gateway-spi v0.0.5 // indirect
	github.com/alibabacloud-go/debug v1.0.1 // indirect
	github.com/aliyun/credentials-go v1.4.5 // indirect
	github.com/clbanning/mxj/v2 v2.7.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/tjfoc/gmsm v1.4.1 // indirect
	golang.org/x/net v0.50.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/e2b-dev/infra/packages/api => ../api
	github.com/e2b-dev/infra/packages/shared => ../shared
)
