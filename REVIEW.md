# Runner v1 Review (Spec v0.1 + IMPLEMENTATION_PLAN)

## Escopo

Revisão baseada em:

1. Spec v0.1: https://github.com/duckflux/spec
2. Plano local: `IMPLEMENTATION_PLAN.md`

Critério solicitado: validar entrega dos objetivos de v1 (excluindo roadmap futuro/v2).

Código revisado: `cmd/duckflux`, `internal/*`, `schema/*`, `examples/*`, CI/Makefile e suíte de testes.

## Resultado geral

A base v1 está **majoritariamente implementada**: parser + schema, validação semântica, engine (sequencial/loop/parallel/if/when), timeouts, onError (fail/skip/retry/redirect), participantes principais (`exec`, `http`, `human`, `workflow`), CLI (`run/lint/validate/version`) e testes de unidade/integração.

A suíte atual passa integralmente:

- `go test ./...` -> OK em todos os pacotes.

Mesmo assim, encontrei **gaps relevantes** de aderência ao spec/plano.

## Gaps e problemas (priorizados)

### 1) `lint` não compila todas as expressões CEL de `input` (erro só aparece em runtime)

**Severidade:** Alta

**Evidência de código:**

- [internal/parser/validate.go](internal/parser/validate.go):117 compila `when`, mas não compila `override.input`.
- [internal/parser/validate.go](internal/parser/validate.go):181 só compila `participant.input` quando ele é `map[string]interface{}` e apenas no 1o nível.
- [internal/engine/sequential.go](internal/engine/sequential.go):121 `input` é compilado/avaliado no runtime.

**Comportamento observado:**

- `duckflux lint` retorna `OK` para workflow com `input: "!!!bad{{"`.
- `duckflux run` falha depois com erro de compilação CEL.

**Impacto:** quebra da expectativa de "falhar cedo" no lint para CEL inválida.

**Recomendação:** no `ValidateSemantic`, compilar CEL recursivamente para qualquer `input` de participante e de override (string e mapas aninhados).

### 2) `if`, `when` e `loop.until` aceitam resultado não booleano silenciosamente

**Severidade:** Alta

**Evidência de código:**

- [internal/engine/sequential.go](internal/engine/sequential.go):106 (`when`): `if cond, _ := result.(bool); !cond { ... }`
- [internal/engine/control.go](internal/engine/control.go):69 (`until`)
- [internal/engine/control.go](internal/engine/control.go):129 (`if`)

Nos três casos, se a expressão retorna algo não-bool (ex.: `"1"`), o cast falha e o fluxo segue como `false`, sem erro.

**Impacto:** comportamento inesperado e difícil de depurar; condição malformada não é rejeitada.

**Recomendação:** exigir tipo booleano explicitamente e retornar erro quando o resultado não for `bool`.

### 3) `run` não valida schema de inputs declarados (required/type/format)

**Severidade:** Alta

**Evidência de código:**

- [cmd/duckflux/main.go](cmd/duckflux/main.go):197 (`runWorkflow`) resolve inputs e executa, sem chamar `parser.ValidateInputs`.
- [cmd/duckflux/main.go](cmd/duckflux/main.go):98 (`validate`), por outro lado, chama `parser.ValidateInputs`.

**Comportamento observado:** workflow com input `required: true` pode rodar sem fornecer input, se esse campo não for usado na execução.

**Impacto:** divergência entre "workflow válido para executar" e "workflow validado"; requisitos de input podem ser ignorados em runtime.

**Recomendação:** no `run`, chamar `parser.ValidateInputs(wf, inputs)` antes de `engine.Run`.

### 4) Superfície de variáveis runtime incompleta vs spec (metadata de step)

**Severidade:** Média/Alta

**Evidência de código:**

- [internal/cel/variables.go](internal/cel/variables.go):21 `StepResult` só expõe `output`, `status`, `retries`.
- [internal/cel/env.go](internal/cel/env.go):89 bindings de step só incluem esses 3 campos.

No spec v0.1, a seção de variáveis runtime lista também `startedAt`, `finishedAt`, `duration`, `error` para steps.

**Comportamento observado:** expressão como `step.startedAt` falha com `no such key`.

**Impacto:** workflows/specs que dependem desses metadados não funcionam.

**Recomendação:** ampliar `StepResult` e preencher esses campos no engine (inclusive em erro/skip).

### 5) `workflow.path` de sub-workflow é aberto sem resolução relativa ao arquivo pai

**Severidade:** Média

**Evidência de código:**

- [cmd/duckflux/main.go](cmd/duckflux/main.go):323 usa `os.Open(path)` diretamente.

O caminho é interpretado a partir do CWD do processo, não da pasta do workflow pai.

**Impacto:** workflows com caminhos relativos podem quebrar dependendo de onde o binário é executado.

**Recomendação:** resolver `path` relativo ao diretório do workflow chamador (incluindo chamadas recursivas).

### 6) Logging estruturado por step (start/end/duration/status) não foi entregue

**Severidade:** Média

**Evidência de plano:** `IMPLEMENTATION_PLAN.md` pede logs de step (`Phase 6`, item de `slog`).

**Evidência de código:**

- [cmd/duckflux/main.go](cmd/duckflux/main.go):232 e :237 logam só início/fim do workflow.
- Não há logging de step no engine.

**Impacto:** observabilidade limitada para troubleshooting.

**Recomendação:** instrumentar `runParticipantStep` e handlers de controle com eventos de início/fim, duração e status.

### 7) Inconsistência `hook`: plano/model suportam stub, schema rejeita tipo

**Severidade:** Baixa

**Evidência de código/plano:**

- [internal/model/participant.go](internal/model/participant.go):13 inclui `ParticipantTypeHook`.
- `IMPLEMENTATION_PLAN.md` (Issue 14/Phase 5e) pede stub `hook`.
- [schema/duckflux.schema.json](schema/duckflux.schema.json):58 enum não inclui `hook`.

**Impacto:** implementação de stub existe, mas não é utilizável via YAML validado por schema.

**Recomendação:** alinhar plano/model/schema (ou remover `hook` do escopo v1 de forma consistente).

## Objetivos v1 atendidos (resumo)

Atendidos de forma sólida:

- Estrutura do projeto, módulo Go, Makefile e CI.
- Modelos principais e parsing de `FlowStep` union.
- Parser com validação de schema + validação semântica básica.
- CEL environment/eval com variáveis principais.
- Engine com execução sequencial + controle (`loop`, `parallel`, `if`, `when`).
- Timeout e pipeline de erro (`fail`, `skip`, `retry`, redirect).
- Participantes `exec`, `http`, `human`, `workflow` + stubs `agent/mcp/hook`.
- CLI `run/lint/validate/version`.
- Exemplos e integração end-to-end (`exec` + `http`).

## Conclusão

A entrega v1 está funcional e com boa cobertura de testes, mas **não está 100% aderente** aos objetivos combinados de spec/plano.

Os itens 1-4 acima são os principais para fechar lacunas de comportamento e conformidade sem avançar para roadmap v2.
