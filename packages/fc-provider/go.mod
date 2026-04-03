module github.com/e2b-dev/infra/packages/fc-provider

go 1.22

require (
	github.com/alibabacloud-go/darabonba-openapi/v2 v2.0.10
	github.com/alibabacloud-go/fc-20230330-3 v3.0.8
	github.com/alibabacloud-go/tea-utils/v2 v2.0.7
	github.com/e2b-dev/infra/packages/api v0.0.0
)

replace github.com/e2b-dev/infra/packages/api => ../api