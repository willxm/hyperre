# hyperre

[English](README.md) | 简体中文

**面向海量正则的高性能匹配库：用一次 AC 自动机扫描，代替逐条执行成千上万条正则。**

`hyperre` 从每条正则中提取“必须出现”的字面量特征串，放进同一个 Aho-Corasick 自动机；
匹配时只扫描一次文本，只执行特征串命中的正则。结果与 Go `regexp` 逐条匹配完全一致，
规则多时通常快 **10–15 倍**。

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

合成的类规则正则（中英文词、`.{0,5}`、`(?i)` 分支、字符类，约 5% 无特征串），300 字节文本，2 核 Intel Xeon @ 2.8 GHz，Go 1.24：

| 正则数 | 逐条匹配 | `FindAll` | 加速 | 实际执行数 |
|---:|---:|---:|---:|---:|
| 100 | 448 µs | 44 µs | 10× | 6 |
| 1,000 | 4.5 ms | 0.33 ms | 14× | 50 |
| 10,000 | 52.7 ms | 4.1 ms | 13× | 504 |

`New` 约 20 ms / 千条（主要是 `regexp.Compile`）。

## 局限

- Go RE2 语义，不支持环视和反向引用。
- 没有特征串的正则每次都会执行；给它们加上有区分度的字面量（如把 `\d{11}` 改为 `订单\d{11}`）是最有效的优化。

## License

MIT
