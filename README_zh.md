# hyperre

[English](README.md) | 简体中文

**面向海量正则的高性能匹配库：用一次 AC 自动机扫描，代替逐条执行成千上万条正则。**

`hyperre` 从每条正则中提取“必须出现”的字面量特征串，放进同一个 Aho-Corasick 自动机；
匹配时只扫描一次文本，只执行特征串命中的正则。结果与 Go `regexp` 逐条匹配完全一致，
典型规则集快 **13–15 倍**，规则都有特征串时可快上千倍。

纯 Go，仅依赖标准库，Go 1.18+。名字致敬 Intel 的 [Hyperscan](https://github.com/intel/hyperscan)（同类问题的 C 语言方案）。

```sh
go get github.com/willxm/hyperre
```

## 使用

### 正则列表

```go
m, err := hyperre.New([]string{
    `(?i)buy\s+cheap\s+\w+`,
    `敏感.{0,3}词`,
    `1[3-9]\d{9}`,
})

m.Match(text)   // bool：是否命中任意一条
m.FindAll(text) // []string：命中的正则
```

### 自定义规则

直接传入你自己的规则类型，告诉它正则在哪个字段；命中结果就是你的规则本身，所有自定义字段都在：

```go
type Rule struct {
    ID     int
    Expr   string
    Action string
}

m, err := hyperre.New(rules, hyperre.WithPattern(func(r Rule) string { return r.Expr }))

for _, r := range m.FindAll(text) { // []Rule
    fmt.Println(r.ID, r.Action)
}
```

### 方法

| | |
|---|---|
| `Match(text) bool` | 是否命中任意一条（命中即停） |
| `Find(text) (T, bool)` | 第一条命中的规则 |
| `FindAll(text) []T` | 全部命中的规则，按原顺序 |
| `FindAllIndex(text) []Hit[T]` | 同上，并附带规则下标和命中位置（`Loc`） |
| `Filter(text) []T` | 只做预过滤：可能命中的规则，不执行正则 |
| `Len() int` | 规则数 |

### 可选参数

| Option | 默认 | 说明 |
|---|---|---|
| `WithPattern(func(T) string)` | — | 规则的正则在哪里（`[]string` 不需要） |
| `WithIgnoreCase()` | 关 | 所有正则忽略大小写 |
| `WithOnInvalid(func(i int, pattern string, err error))` | 关 | 跳过编译失败的正则并回调，而不是返回错误 |
| `WithMinLiteralLen(n)` | 2 | 用于索引的特征串最短长度 |
| `WithCJKSingleChar(bool)` | true | 允许单个中日韩字符作为特征串（`敏.*词` → `敏`） |
| `WithMaxLiterals(n)` | 1024 | 单条正则最多特征串数，超过（大字符类 / 分支）则每次都执行 |

后三个只影响索引（性能），不影响匹配结果。

### 并发与更新

`Matcher` 不可变，可并发使用。更新规则时重新构建一个再替换：

```go
var current atomic.Pointer[hyperre.Matcher[Rule]]

current.Store(hyperre.MustNew(rules, opt)) // 加载 / 重载时
current.Load().FindAll(text)                 // 每次请求
```

## 正确性保证

> 只要 Go `regexp` 认为某条正则能匹配文本，Matcher 一定返回它，`Filter` 一定包含它。

预过滤只跳过“可证明不可能命中”的正则；提取不到可用特征串的正则（`a.*b`、`\d{11}`、`[a-z]+`）每次都执行。
由单元测试、在所有参数组合下与 `regexp` 逐条对比的随机测试（数十万组）和 fuzz 测试共同保证。

## 原理

用与 `regexp.Compile` 相同的 flag 解析正则 AST 并递归提取，每个节点返回一组特征串，满足“该节点的任何匹配都至少包含其中之一”：

| AST 节点 | 示例 | 提取结果 | 安全性 |
|---|---|---|---|
| Literal | `abc` | `{abc}` | 必然出现在匹配中 |
| CharClass | `[中美]` | `{中, 美}` | 匹配必是其中某个字符 |
| Concat | `foo.*bar` | 在 `{foo}`、`{bar}` 中选最优 | 每部分都必然出现，选区分度最高的 |
| Alternate | `abc\|def` | `{abc, def}` | 所有分支都能提取时才成立 |
| Capture / `+` / `{n,}`(n≥1) | `(abc)+` | 子表达式结果 | 至少出现一次 |
| `?` `*` `{0,n}` 锚点 `.` 等 | `a?` | 无 | 不保证出现任何字符 |

**大小写折叠**：特征串与文本统一归一化为 `unicode.SimpleFold` 的规范 key，与 `regexp/syntax` 处理 `(?i)` 的方式一致。
简单的 `ToLower` 会漏掉 `(?i)ss` 对 `ſſ`、`(?i)σα` 对 `ςα`、`(?i)вв` 对 `ᲀᲀ` 等匹配。

**自动机**：AC 自动机压平为三个不含指针的连续数组，内存紧凑、对 GC 友好。

## 性能

以下为 `go run ./benchmarks` 的实测数据：单线程（GOMAXPROCS=1），Apple Silicon Mac（Linux arm64 虚拟机，4 vCPU，Go 1.24.7）。
规则为生成的类业务正则：带间隔的中英文词（`w1.{0,5}w2`）、忽略大小写的分支（`(?i)(w1|w2)\s*w3`）、字符类（`w1[\p{Han}a-z]{0,3}w2`）；
每条文本约 300 字节中英混合，其中 30% 含命中。每次运行都会把 hyperre 的结果与 `regexp` 逐条核对。

“regexp 逐条”指对每个编译好的 `regexp.Regexp` 依次匹配；“合并正则”指常见的 `(p1)|(p2)|…` 合成一个大正则的做法。

### 典型规则集：5% 的规则没有特征串

像 `\d{11}` 这类规则无法索引，每条文本都要执行，它们决定了性能下限。

**FindAll**（返回全部命中规则），每条文本耗时：

| 规则数 | regexp 逐条 | hyperre | 加速 | hyperre 实际执行 |
|---:|---:|---:|---:|---:|
| 100 | 128.2 µs | 9.0 µs | **14×** | 3（3.3%） |
| 1k | 1.27 ms | 87.6 µs | **15×** | 45（4.5%） |
| 10k | 12.62 ms | 982 µs | **13×** | 525（5.2%） |
| 100k | 128.87 ms | 9.78 ms | **13×** | 4,957（5.0%） |

**Match**（是否命中任意一条），每条文本耗时：

| 规则数 | regexp 逐条 | 合并正则 | hyperre | 对比逐条 | 对比合并 |
|---:|---:|---:|---:|---:|---:|
| 100 | 109.6 µs | 226.7 µs | 8.3 µs | **13×** | 27× |
| 1k | 1.05 ms | 3.48 ms | 72.2 µs | **15×** | 48× |
| 10k | 10.91 ms | 52.66 ms | 839.8 µs | **13×** | 63× |
| 100k | 107.66 ms | 2.24 s | 7.67 ms | **14×** | 292× |

### 所有规则都有特征串

同一生成器加 `-fallback 0`（例如敏感词表）。AC 扫描耗时几乎不随规则数增长，而逐条匹配线性增长：

| 规则数 | regexp 逐条（FindAll） | hyperre（FindAll） | 加速 | 合并正则（Match） | hyperre（Match） |
|---:|---:|---:|---:|---:|---:|
| 100 | 158.1 µs | 3.5 µs | **45×** | 256.2 µs | 3.5 µs |
| 1k | 1.28 ms | 4.7 µs | **272×** | 3.69 ms | 4.8 µs |
| 10k | 12.91 ms | 6.0 µs | **2,149×** | 57.92 ms | 5.8 µs |
| 100k | 131.44 ms | 18.2 µs | **7,218×** | 2.43 s | 15.9 µs |

### 构建与内存（5% 规则集）

| 规则数 | `regexp.Compile` 全部 | `hyperre.New` | 堆内存（regexp） | 堆内存（hyperre） |
|---:|---:|---:|---:|---:|
| 100 | 0.31 ms | 1.28 ms | 0.2 MB | 0.3 MB |
| 1k | 3.38 ms | 5.43 ms | 1.6 MB | 1.7 MB |
| 10k | 128 ms | 137 ms | 15.7 MB | 17.0 MB |
| 100k | 657 ms | 740 ms | 157.7 MB | 167.8 MB |

`hyperre.New` 已包含编译全部正则的耗时；索引额外占用约 6–10% 内存。

**结论**

- hyperre 的开销 = 一次文本扫描（几微秒，几乎与规则数无关）+ 无法索引的规则。5% 无特征串时，各规模稳定快 13–15 倍；
  全部可索引时，差距随规则数扩大，10 万条规则下超过 7000 倍。
- “合并成一个大正则”比逐条匹配还慢，规模越大越糟（10 万条时每条文本 2.2 秒）。
- 最有效的优化：给没有特征串的规则加上有区分度的字面量（如把 `\d{11}` 改为 `订单\d{11}`）。

在你自己的机器上复现：

```sh
go run ./benchmarks                       # 100、1k、10k、100k 四种规模
go run ./benchmarks -fallback 0           # 所有规则都有特征串
go run ./benchmarks -sizes 1000 -budget 5s
```

## 局限

- Go RE2 语义，不支持环视和反向引用。
- 没有特征串的正则每次都会执行；给它们加上有区分度的字面量（如把 `\d{11}` 改为 `订单\d{11}`）是最有效的优化。

## License

MIT
