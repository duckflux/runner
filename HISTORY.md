# History

## Decisões passadas

### 1. Arquitetura em pacotes `internal/` com separação clara de responsabilidades
**PR:** [#1](https://github.com/duckflux/runner/pull/1) · **Por:** @ggondim  
O projeto foi estruturado como um módulo Go usando `internal/` com cinco pacotes de responsabilidade única: `model`, `parser`, `cel`, `engine`, `participant`. A CLI usa Cobra com os subcomandos `run`, `lint` e `validate`. Dependências externas limitadas a: `cel-go`, `yaml.v3`, `jsonschema/v6`, `cobra`.

---

### 2. `FlowStep` como tipo union com `UnmarshalYAML` customizado
**PR:** [#1](https://github.com/duckflux/runner/pull/1) · **Por:** @ggondim  
O tipo `FlowStep` é um union que despacha para o tipo concreto correto (exec, http, loop, parallel, if, workflow, etc.) com base no formato do nó YAML durante o unmarshalling. Isso mantém a API do modelo limpa sem exigir wrappers discriminadores explícitos no YAML dos usuários.

---

### 3. `State` único como contexto de avaliação CEL
**PR:** [#1](https://github.com/duckflux/runner/pull/1) · **Por:** @ggondim  
Um único struct `State` serve como contexto de avaliação CEL para todo o workflow. Os resultados de cada step são indexados pelo nome do participante e sobrescritos a cada re-execução em loops. Isso simplifica o model de estado ao custo de não manter histórico de iterações anteriores dentro de um loop.

---

### 4. Cadeia de resolução de timeout: `flow > participant > defaults > none`
**PR:** [#1](https://github.com/duckflux/runner/pull/1), [#26](https://github.com/duckflux/runner/pull/26) · **Por:** @ggondim (decisão), @copilot (implementação)  
Timeouts são resolvidos numa cadeia de prioridade decrescente: override no flow step > declaração no participant > defaults globais > nenhum timeout. Falhas de timeout passam pelo mesmo pipeline `onError`, já que `context.DeadlineExceeded` se manifesta como um erro normal do `Execute`.

---

### 5. Pipeline `onError`: fail → skip → retry → redirect
**PR:** [#1](https://github.com/duckflux/runner/pull/1), [#27](https://github.com/duckflux/runner/pull/27) · **Por:** @ggondim (decisão), @copilot (implementação)  
O pipeline de tratamento de erros segue a ordem: `fail` (padrão) → `skip` → `retry` (com backoff exponencial e cancelamento de contexto) → `redirect` para um participante nomeado. O retry implementa backoff exponencial respeitando o cancelamento de contexto.

---

### 6. `parallel:` mapeado para goroutines + `sync.WaitGroup` com cancelamento em falha
**PR:** [#1](https://github.com/duckflux/runner/pull/1), [#25](https://github.com/duckflux/runner/pull/25) · **Por:** @ggondim (decisão), @copilot (implementação)  
Branches paralelos são executados como goroutines coordenadas por `sync.WaitGroup`. Se qualquer branch falhar, o contexto compartilhado é cancelado, interrompendo os demais branches em andamento. O `State` tem `sync.RWMutex` para escrita thread-safe de resultados de steps.

---

### 7. `agent`, `mcp` e `hook` são stubs na v1
**PR:** [#1](https://github.com/duckflux/runner/pull/1), [#32](https://github.com/duckflux/runner/pull/32) · **Por:** @ggondim  
Os tipos de participante `agent`, `mcp` e `hook` foram intencionalmente deixados como stubs que retornam `"not yet implemented"` na v1. Cada stub define uma interface v2 tipada para guiar implementações futuras, mantendo conformidade com o schema JSON.

---

### 8. Reescrita interna de `loop` → `_loop` para evitar conflito com palavra reservada do CEL
**PR:** [#21](https://github.com/duckflux/runner/pull/21) · **Decisão:** @ggondim · **Implementação:** @copilot  
`loop` é um identificador reservado no CEL. Em vez de expor esse detalhe de implementação aos desenvolvedores de workflows, o runner reescreve transparentemente `loop.` para `_loop.` antes de compilar qualquer expressão CEL. Expressões como `loop.index` e `loop.first` funcionam naturalmente. A reescrita usa regex com word-boundary para não afetar identificadores que apenas *contêm* "loop" (ex: `myloop.field`).

---

### 9. Solução do problema de importação circular no participante `workflow` via injeção de dependência
**PR:** [#31](https://github.com/duckflux/runner/pull/31) · **Por:** @copilot  
O pacote `engine` já importa `participant`, então `participant/workflow.go` não pode importar `engine` (importação circular). A solução foi injeção de dependência via `SubWorkflowRunnerFunc`: uma função de callback fornecida pela camada de wiring (CLI) que fecha sobre `parser.Parse` e `engine.Run`, mantendo o pacote `participant` completamente livre de imports do `engine`.

---

### 10. Validação de schema JSON com schema embutido via `embed`
**PR:** [#22](https://github.com/duckflux/runner/pull/22) · **Por:** @copilot  
A validação de schema usa `santhosh-tekuri/jsonschema/v6` com o arquivo `duckflux.schema.json` embutido no binário via `//go:embed`. O pipeline de parse executa três fases sequenciais: JSON Schema → YAML decode → verificações semânticas.

---

### 11. Coerção de tipo nos inputs da CLI: strings são parsed para os tipos declarados
**PR:** [#34](https://github.com/duckflux/runner/pull/34) · **Por:** @copilot  
Flags `--input key=value` sempre chegam como strings. O validador de inputs (`ValidateInputs`) realiza coerção de tipo: `"42"` é válido para `integer`, `"true"` para `boolean`, etc. Formatos desconhecidos passam silenciosamente para compatibilidade futura. Em conflito entre `--input-file` e `--input`, o flag `--input` tem precedência.

---

### 12. Campos de observabilidade adicionados ao `StepResult`
**PR:** [#36](https://github.com/duckflux/runner/pull/36) · **Por:** @copilot  
`StepResult` ganhou os campos `startedAt`, `finishedAt`, `duration` e `error`, populados pelo engine a cada execução de step. Logging estruturado via `slog` foi adicionado por step (início/fim/duração/status).

---

### 13. Path de sub-workflows resolvido relativo ao workflow pai
**PR:** [#36](https://github.com/duckflux/runner/pull/36) · **Por:** @copilot  
O campo `path` de um participante do tipo `workflow` é resolvido relativamente ao diretório do workflow que o invoca, não ao diretório de trabalho atual. Isso torna workflows portáveis e previsíveis independentemente de onde o binário é chamado.

---

### 14. Expressões CEL não-booleanas em `if`/`when`/`loop.until` são tratadas como erro
**PR:** [#36](https://github.com/duckflux/runner/pull/36) · **Por:** @copilot  
Se uma expressão CEL em contexto de controle de fluxo (`if`, `when`, `loop.until`) produzir um resultado não-booleano, o engine retorna um erro explícito em vez de fazer coerção silenciosa. Isso previne bugs difíceis de diagnosticar causados por expressões mal formadas.

---

### 15. Plan de build faseado com grafo de dependências otimizado para paralelismo
**PR:** [#1](https://github.com/duckflux/runner/pull/1) · **Por:** @ggondim  
O desenvolvimento foi planejado em fases com dependências explícitas: Phase 0 (bootstrap) → Phase 1 `model` ∥ Phase 2 `cel` (independentes, executáveis em paralelo) → Phase 3 (parser) → Phase 4 (engine: sequential, control flow, timeout, error handling) → Phase 5a–f (participants, todos paralelos entre si) → Phase 6 (CLI, exemplos, testes e2e).

---

## Changelog (issues resolvidas)

| Issue | Título |
|-------|--------|
| [#18](https://github.com/duckflux/runner/issues/18) | Example Workflows & Integration Tests |
| [#17](https://github.com/duckflux/runner/issues/17) | CLI — validate command |
| [#16](https://github.com/duckflux/runner/issues/16) | CLI — run command, input handling, output formatting |
| [#15](https://github.com/duckflux/runner/issues/15) | Participant stubs — agent, mcp, hook |
| [#14](https://github.com/duckflux/runner/issues/14) | Participant — workflow (sub-workflow composition) |
| [#13](https://github.com/duckflux/runner/issues/13) | Participant — human (interactive input) |
| [#12](https://github.com/duckflux/runner/issues/12) | Participant — http (HTTP requests) |
| [#11](https://github.com/duckflux/runner/issues/11) | Participant — exec (shell command execution) |
| [#10](https://github.com/duckflux/runner/issues/10) | Error Handling & Retry |
| [#9](https://github.com/duckflux/runner/issues/9) | Timeout System |
| [#8](https://github.com/duckflux/runner/issues/8) | Execution Engine — Control Flow (loop, parallel, if, when) |
| [#7](https://github.com/duckflux/runner/issues/7) | Execution Engine — State, Sequential Execution, Input/Output Mapping |
| [#6](https://github.com/duckflux/runner/issues/6) | Semantic Validation & Lint Command |
| [#5](https://github.com/duckflux/runner/issues/5) | YAML Parser & JSON Schema Validation |
| [#4](https://github.com/duckflux/runner/issues/4) | CEL Environment & Expression Evaluation |
| [#3](https://github.com/duckflux/runner/issues/3) | Core Model Types — Go structs for the spec schema |
| [#2](https://github.com/duckflux/runner/issues/2) | Project Bootstrap — Go module, directory structure, CI, Makefile |
