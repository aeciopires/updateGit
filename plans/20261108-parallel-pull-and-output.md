# Plano: updateGit 0.2.0 - pull em paralelo, saída, logs e CI

> **Status (2026-11-08): implementado** na versão `0.2.0`, com todas as decisões recomendadas da seção 3. Critérios de aceite na [seção 20](#20-critérios-de-aceite).
>
> **Diferenças entre o plano e a implementação:**
>
> - O logger foi reescrito sem o `zerolog` (dependência removida) em vez de usar `zerolog.MultiLevelWriter`. O formato e as cores são os da seção 4. Em `debug`, ERROR/FATAL mostram `arquivo:linha`, e só o FATAL imprime o stack trace completo.
> - O linter `goprintffuncname` não foi habilitado, porque exigiria renomear o `common.Logger`. O `govet printf` já trata o `Logger` como função printf e barra o `%w`.
> - O `mise.toml` também fixa o `python` (3.14.10): o pre-commit do mise rodava com o Python 3.10 do sistema e falhava.
> - Um repositório em timeout pode levar até 2s além do `--git-timeout`, que é o tempo para o `ssh` liberar a saída depois que o `git` é encerrado (`WaitDelay`).
> - Sem cores, o recuo dos blocos é de 10 colunas (o rótulo mais largo é `[TIMEOUT]`), e não de 7 como no exemplo da seção 8.3.
> - Também entraram no plano a correção do `CLI_DEBUG`/`debug` (não tinham efeito) e a precedência flag > env > arquivo > default, que antes era outra (ver CHANGELOG).
> - Ficam para quem administra o repositório no GitHub: a *branch protection* da `main` exigindo o job `ci` e o *ruleset* de proteção de tags (seção 11.5).
>
> **Ajustes posteriores** (registrados na [seção 21](#21-ajustes-após-a-implementação)): profundidade máxima da busca passou de 3 para 6 (padrão continua 2); criada a pasta `docs/`; repositórios com arquivos não commitados passam a ser pulados por padrão; diagramas validados com Docker (`make lint-diagrams`); revisão de boas práticas e segurança; cobertura de testes de 88,8% para 95,4% (mínimo exigido passou para 90%).
>
> **Teste manual de carga** (50 repositórios, remoto local): `-J 1` = 0,98s, `-J 15` = 0,13s, `-J 50` = 0,11s.

<!-- TOC -->

- [Plano: updateGit 0.2.0 - pull em paralelo, saída, logs e CI](#plano-updategit-020---pull-em-paralelo-saída-logs-e-ci)
  - [1. Objetivo](#1-objetivo)
  - [2. Situação atual (diagnóstico)](#2-situação-atual-diagnóstico)
  - [3. Decisões para aprovar](#3-decisões-para-aprovar)
  - [4. Fase 1 - Logger: cores, nível mínimo e segurança para concorrência](#4-fase-1---logger-cores-nível-mínimo-e-segurança-para-concorrência)
  - [5. Fase 2 - Informações de branch](#5-fase-2---informações-de-branch)
  - [6. Fase 3 - Execução em paralelo](#6-fase-3---execução-em-paralelo)
  - [7. Fase 4 - Saída por repositório e resumo final](#7-fase-4---saída-por-repositório-e-resumo-final)
  - [8. Exemplos de mensagens (antes x depois)](#8-exemplos-de-mensagens-antes-x-depois)
  - [9. Configuração: flags, variáveis de ambiente e arquivo](#9-configuração-flags-variáveis-de-ambiente-e-arquivo)
  - [10. Testes](#10-testes)
    - [10.1 Fase 1 - Logger](#101-fase-1---logger)
    - [10.2 Fase 2 - Branches](#102-fase-2---branches)
    - [10.3 Fase 3 - Paralelismo](#103-fase-3---paralelismo)
    - [10.4 Fase 4 - Saída](#104-fase-4---saída)
    - [10.5 Configuração e flags](#105-configuração-e-flags)
    - [10.6 Fase 5 - Release e CI](#106-fase-5---release-e-ci)
    - [10.7 Testes existentes que mudam](#107-testes-existentes-que-mudam)
    - [10.8 Fase 6 - Checagens de desenvolvimento](#108-fase-6---checagens-de-desenvolvimento)
    - [10.9 Fase 7 - Log em arquivo](#109-fase-7---log-em-arquivo)
    - [10.10 Fase 8 - Barra de progresso](#1010-fase-8---barra-de-progresso)
    - [10.11 Fase 9 - Busca recursiva](#1011-fase-9---busca-recursiva)
    - [10.12 Fase 10 - Concorrência de backups](#1012-fase-10---concorrência-de-backups)
  - [11. Fase 5 - CI de release no GitHub Actions](#11-fase-5---ci-de-release-no-github-actions)
  - [12. Fase 6 - Checagens de desenvolvimento no Makefile e CI de PR](#12-fase-6---checagens-de-desenvolvimento-no-makefile-e-ci-de-pr)
  - [13. Fase 7 - Exportar o log para arquivo](#13-fase-7---exportar-o-log-para-arquivo)
  - [14. Fase 8 - Barra de progresso ao vivo](#14-fase-8---barra-de-progresso-ao-vivo)
  - [15. Fase 9 - Busca recursiva de repositórios](#15-fase-9---busca-recursiva-de-repositórios)
  - [16. Fase 10 - Limite separado de concorrência para backups](#16-fase-10---limite-separado-de-concorrência-para-backups)
  - [17. Documentação, CHANGELOG e CLAUDE.md](#17-documentação-changelog-e-claudemd)
  - [18. Riscos e mitigação](#18-riscos-e-mitigação)
  - [19. Fora do escopo (ideias futuras)](#19-fora-do-escopo-ideias-futuras)
  - [20. Critérios de aceite](#20-critérios-de-aceite)
  - [21. Ajustes após a implementação](#21-ajustes-após-a-implementação)

<!-- TOC -->

## 1. Objetivo

1. Permitir que o `updateGit pull` atualize N repositórios ao mesmo tempo (ex.: 15, 20 ou 50), com limite configurável.
2. Deixar a saída mais intuitiva: cores por tipo de mensagem (DEBUG, INFO, WARNING, ERROR), blocos por repositório e um resumo final.
3. Criar um parâmetro para escolher o nível mínimo de log.
4. Deixar fácil ver a branch atual e as outras branches locais de cada repositório.
5. Automatizar a release: a cada tag de versão semântica, o GitHub Actions compila os binários para os sistemas operacionais e arquiteturas suportados e publica tudo na página de release do GitHub.
6. Pegar erros durante o desenvolvimento: todos os testes e checagens (format, lint, vet, módulos, vulnerabilidades, workflows) rodando por targets do Makefile, em hooks de git e no CI de pull requests.
7. Exportar o log para um arquivo (default em `/tmp`, com data e hora no nome).
8. Mostrar uma barra de progresso ao vivo durante a atualização.
9. Encontrar repositórios em até 3 níveis de subdiretórios (padrão: 2), sem entrar em pastas que já são repositórios, com a busca completa **antes** de qualquer atualização.
10. Limitar a quantidade de backups simultâneos separadamente da quantidade de `git pull` simultâneos.

> **Definição de pronto (vale para cada fase):** uma fase só termina quando, no mesmo PR/commit, tiver:
>
> 1. o código;
> 2. os **testes** da fase ([seção 10](#10-testes)), passando em `make check` (gofmt, vet e testes com `-race`);
> 3. a **documentação** afetada atualizada (`README.md`, `CONTRIBUTING.md`, `.updateGit.yaml`) ([seção 17](#17-documentação-changelog-e-claudemd));
> 4. a entrada da fase no **`CHANGELOG.md`**, na seção `0.2.0`;
> 5. o **`CLAUDE.md`** atualizado com arquivos novos, convenções e regras que a fase criar;
> 6. o checkbox da fase marcado nos [critérios de aceite](#20-critérios-de-aceite) deste plano.

**Ordem de implementação recomendada.** Os números das fases identificam o assunto; a ordem abaixo respeita as dependências entre elas:

| Ordem | Fase | Motivo |
|---|---|---|
| 1 | Fase 6 - Checagens de desenvolvimento e CI de PR | Primeiro, para que todas as outras fases já sejam validadas por lint, testes com `-race` e CI. |
| 2 | Fase 1 - Logger | Pré-requisito do paralelismo (*data race*) e do log em arquivo. |
| 3 | Fase 7 - Log em arquivo | Usa o `InitLogger` da fase 1. |
| 4 | Fase 2 - Branches | Independente; o resultado é usado pela saída. |
| 5 | Fase 9 - Busca recursiva | Define a etapa de descoberta (que termina antes de qualquer atualização) e a identificação dos repositórios (caminho relativo), usadas no paralelismo, no backup, na barra de progresso e nos logs. |
| 6 | Fase 3 - Paralelismo | Núcleo da mudança. |
| 7 | Fase 10 - Concorrência de backups | Depende do pool de workers. |
| 8 | Fase 4 - Saída | Depende do `RepoResult` da fase 3. |
| 9 | Fase 8 - Barra de progresso | Depende da goroutine de saída da fase 4. |
| 10 | Fase 5 - CI de release | Por último, para publicar a `0.2.0` com tudo pronto. |

## 2. Situação atual (diagnóstico)

| Item | Situação atual | Impacto |
|---|---|---|
| Paralelismo | As flags `-P/--git-parallel-enabled` e `-J/--git-max-concurrent` e as chaves `git.parallel_enabled` e `git.max_concurrent` existem e o README documenta o uso, mas `git.UpdateRepositoriesWithConfig` (`internal/git/git.go`) **ignora** `cfg.Parallel` e processa um repositório por vez. | O usuário acha que roda em paralelo, mas não roda. |
| Timeout | `ParallelUpdateConfig.Timeout` é montado em `cmd/pull.go` (`config.Timeout = 30`), mas nunca é usado. | Um `git pull` travado prende a execução inteira. |
| Logger | `common.Logger` recria `log.Logger` e chama `zerolog.SetGlobalLevel` **a cada chamada**. | Com goroutines isso vira *data race* (o `go test -race` acusa) e desperdiça processamento. |
| Saída do git | `PullRepository` liga o `Stdout`, o `Stderr` e o `Stdin` do git direto no terminal. | Em paralelo, a saída de vários repositórios se mistura e os prompts de senha disputam o mesmo terminal. |
| Nível de log | Só existe `-D/--debug` (liga ou desliga DEBUG). | Não dá para ver só WARNING/ERROR. |
| Cores | O `zerolog.ConsoleWriter` está com `FormatLevel` customizado, sem cor. | Todos os níveis têm a mesma aparência. |
| Erros | ERROR/FATAL imprimem `error=... stack=[{...}]` com o stack trace inteiro. | Poluído para o usuário final. |
| Branches | A branch atual aparece no meio do texto (`on branch 'main'`) e a lista de branches (`git branch` cru) só aparece em DEBUG. | É difícil saber em qual branch cada repositório está. |
| Resultado final | Só uma linha `total=... success=... errors=...`. Com erro, termina em `fatal`, sem listar quais repositórios falharam. | O usuário precisa rolar o log para achar as falhas. |
| Validação | `max_concurrent` só tem `omitempty,number`. | Aceita 0, valores negativos ou 10000. |
| Release | Não existe `.github/workflows`. A release é manual: `make build` local e upload dos arquivos de `bin/` na página do GitHub (`CONTRIBUTING.md`). | Depende da máquina de quem publica e é fácil esquecer um binário ou o `checksums.txt`, o que quebra o `updateGit update`. |
| Log em arquivo | Os logs só vão para o stdout. | Em execuções agendadas (cron), o histórico se perde, a menos que o usuário redirecione a saída manualmente. |
| Progresso | Não há indicação de quanto falta. | Com 50+ repositórios, o usuário não sabe se o programa travou. |
| Busca de repositórios | `FindRepositories` lê só o primeiro nível do `base_dir` (`os.ReadDir`), e `IsGitRepository` exige `.git` como diretório. | Repositórios organizados em pastas (ex.: `~/git/github/repo`) não são encontrados, nem *worktrees* (`.git` é arquivo). |
| Backups | O backup roda dentro do mesmo fluxo do pull, sem limite próprio. | Com paralelismo, 50 cópias `copy` simultâneas saturariam o disco. |
| Checagens de desenvolvimento | O `make check` cobre gofmt, vet e testes. Não há linter (golangci-lint), checagem de `go.mod` (`tidy`/`verify`), busca de vulnerabilidades (govulncheck/trivy), lint dos workflows, hooks de git nem CI de PR. | Erros comuns (erro não tratado, variável não usada, dependência vulnerável, `go.mod` sujo) só aparecem tarde ou nunca. |
| `LDFLAGS` | O `Makefile` injeta `-X main.Version`, `main.Commit` e `main.Date`, mas essas variáveis não existem no `main.go`. A versão real vem de `config.CLIVersion`. | Os `-X` não têm efeito, e o commit e a data do build não aparecem no `-V`. |

## 3. Decisões para aprovar

As recomendações estão marcadas com **(recomendado)**.

1. **Paralelo por padrão?** Hoje o default no código já é `parallel_enabled: true` e `max_concurrent: 10`, só que não tem efeito. Ao implementar, o comportamento padrão passa a ser paralelo.
   - **(recomendado)** Manter `true` com 10 workers e registrar no CHANGELOG como mudança de comportamento.
   - Alternativa: mudar o default para `false` e manter o comportamento atual (sequencial).
2. **Limite máximo de `--git-max-concurrent`:** **(recomendado)** de 1 a 100 (validação `min=1,max=100`). Se o valor for maior que a quantidade de repositórios, ele é reduzido automaticamente.
3. **Ordem dos blocos na saída em paralelo:** **(recomendado)** cada bloco é impresso quando o repositório termina, com contador `[n/N]`, e o resumo final sai em ordem alfabética. Alternativa: esperar todos terminarem e imprimir em ordem alfabética, o que dá menos feedback durante a execução.
4. **Símbolos Unicode (`✔ ✘ ● ⏭`)**: **(recomendado)** usar símbolos quando a saída é um terminal e trocar por `[OK] [FAIL] * [SKIP]` com `--color=never` ou quando a saída é redirecionada para um arquivo ou pipe.
5. **Versão:** são novas funcionalidades compatíveis, então **(recomendado)** `0.2.0`.
6. **Formato da tag:** **(recomendado)** versão semântica **sem** o prefixo `v` (`0.2.0`), como a tag existente `0.1.0` e a saída de `go run . -v`. É obrigatório: o `updateGit update` compara `release.TagName` com `config.CLIVersion`, então uma tag `v0.2.0` faria todo usuário da `0.2.0` ver sempre uma "nova versão".
7. **Pré-releases** (`0.2.0-rc.1`): **(recomendado)** aceitar e publicar como *pre-release* no GitHub. O endpoint `releases/latest` usado pelo `updateGit update` ignora pré-releases, então elas não chegam aos usuários pelo self-update.
8. **`LDFLAGS`:** **(recomendado)** apontar os `-X` para variáveis novas `config.CLICommit` e `config.CLIBuildDate`, exibidas no `updateGit -V`. Alternativa: remover os `-X`.
9. **Nome do arquivo de log:** o pedido é ano, mês, dia e hora no nome. A `/` não pode ser usada em nome de arquivo, então a data usa `-`.
   - **(recomendado)** `/tmp/updateGit_2026-11-08_10-15-02.log`: data e hora com minutos e segundos. Cada execução gera um arquivo próprio e nada se mistura.
   - Alternativa literal: `/tmp/updateGit_2026-11-08_10.log`, só com a hora. Execuções na mesma hora escrevem no mesmo arquivo (modo *append*, com um cabeçalho separando cada execução).
10. **Nível de log do arquivo:** **(recomendado)** o mesmo de `--log-level` por padrão, com a opção `log.file.level` para gravar mais detalhes no arquivo (ex.: `debug` no arquivo e `info` na tela).
11. **Profundidade da busca:** a busca olha **no máximo 3 níveis** a partir do `base_dir`, nunca mais fundo, e não entra em pastas que já são repositórios. Valores aceitos em `--git-max-depth`/`-R`: `1`, `2` ou `3`.
    - **Default `2`** (níveis 1 e 2), como definido.
    - `-R 3` estende a busca ao terceiro nível (ex.: `~/git/github/<org>/<repo>`).
    - `-R 1` volta ao comportamento atual (só o primeiro nível).
    - **Mudança de comportamento com o default `2`:** pastas comuns no primeiro nível (ex.: `~/git/github/`) passam a ser exploradas e os repositórios dentro delas são encontrados.
12. **Barra de progresso:** **(recomendado)** implementação própria (~150 linhas) com `golang.org/x/term` para a largura do terminal, em vez de uma biblioteca pronta. A barra precisa coordenar com os blocos e o logger na mesma saída, e bibliotecas prontas costumam disputar o terminal.
13. **Checagens no pre-commit:** **(recomendado)** usar o [pre-commit](https://pre-commit.com) (já disponível no mise) com hooks locais que chamam os targets do Makefile: rápidos no `pre-commit` e completos no `pre-push`.

## 4. Fase 1 - Logger: cores, nível mínimo e segurança para concorrência

Esta fase é pré-requisito do paralelismo por causa do *data race*.

**Arquivos:** `internal/common/common.go`, `internal/config/config.go`, `cmd/root.go`.

1. Criar `common.InitLogger(level string, color string)`, chamado uma vez no `initConfig()` depois do merge da configuração (flags + env + arquivo):
   - Configura `log.Logger` e `zerolog.SetGlobalLevel` **uma única vez**.
   - O `Logger(level, msg, args...)` continua com a mesma assinatura, então as chamadas existentes não mudam. Ele passa só a formatar e emitir a mensagem, sem reconfigurar nada.
   - Proteção: se o `Logger` for chamado antes do `InitLogger` (por exemplo, nos logs de debug do próprio `initConfig`), usar um `sync.Once` com configuração padrão (`info`, `auto`).
2. **Nível mínimo** (`--log-level`/`-L`, valores `debug|info|warning|error`, default `info`):
   - `-D/--debug` continua funcionando e equivale a `--log-level=debug`, com precedência sobre `--log-level` para não quebrar scripts existentes.
   - Valor inválido gera erro de validação claro (`oneof=debug info warning error`).
   - `fatal` sempre aparece, independente do nível.
3. **Cores** (`--color`, valores `auto|always|never`, default `auto`):
   - `auto` usa cor só quando o stdout é um terminal (`os.Stdout.Stat()` com `ModeCharDevice`) **e** a variável `NO_COLOR` não está definida ([no-color.org](https://no-color.org)).
   - Paleta por nível:

     | Nível | Rótulo | Cor do rótulo | Cor da mensagem |
     |---|---|---|---|
     | DEBUG | `DEBUG` | cinza (`90`) | cinza |
     | INFO | `INFO ` | ciano (`36`) | padrão do terminal |
     | WARNING | `WARN ` | amarelo negrito (`1;33`) | amarelo |
     | ERROR | `ERROR` | vermelho negrito (`1;31`) | vermelho |
     | FATAL | `FATAL` | branco sobre fundo vermelho (`1;37;41`) | vermelho |

   - Rótulos com largura fixa de 5 caracteres para alinhar as colunas.
   - Timestamp curto (`15:04:05`) em cinza. O formato completo (`2006-01-02 15:04:05`) fica para o nível `debug`.
4. **Stack trace** só aparece com `--log-level=debug`. Nos outros níveis, ERROR/FATAL mostram apenas a mensagem (e `arquivo:linha` em debug).
5. Remover o loop de debug com `reflect` do `Execute()` (`cmd/root.go`), que duplica o dump YAML já feito em `initConfig`.

## 5. Fase 2 - Informações de branch

**Arquivo:** `internal/git/git.go`.

1. Nova estrutura e função:

   ```go
   type BranchInfo struct {
       Name     string // nome da branch local
       Current  bool   // é a branch do HEAD
       Upstream string // ex.: origin/main ("" se não houver)
       Ahead    int    // commits locais não enviados
       Behind   int    // commits remotos não baixados
       Gone     bool   // upstream foi apagado no remoto
   }

   type RepoBranches struct {
       Current  *BranchInfo   // nil em detached HEAD
       Detached string        // SHA curto quando HEAD está destacado
       Others   []BranchInfo  // demais branches locais (ordem alfabética)
   }

   func GetRepoBranches(repoPath string) (RepoBranches, error)
   ```

2. Um único comando git, com saída estruturada e sem parsing frágil do `git branch`:

   ```bash
   git for-each-ref --format='%(HEAD)|%(refname:short)|%(upstream:short)|%(upstream:track,nobracket)' refs/heads
   ```

   O detached HEAD é detectado quando nenhuma linha tem `*`. Nesse caso, `git rev-parse --short HEAD` informa o SHA.
3. As informações são coletadas **depois** do pull, para que o `ahead/behind` reflita o estado atualizado.
4. Exibição (detalhes na seção 8):
   - Branch atual com `●` e em verde negrito, seguida do upstream e do estado (`up to date`, `ahead 2`, `behind 3`, `no upstream`, `upstream gone`).
   - Outras branches em uma linha, separadas por vírgula. As que estão `ahead` (com commits não enviados) aparecem em amarelo, para lembrar o usuário.
   - Mais de 8 branches: mostra 8 e `+N more`. A lista completa aparece com `--log-level=debug`.
5. `GetCurrentBranch` e `GetBranches` continuam existindo, por compatibilidade e por causa dos testes, mas o fluxo de pull passa a usar `GetRepoBranches`.

## 6. Fase 3 - Execução em paralelo

**Arquivos:** `internal/git/git.go` (ou um novo `internal/git/runner.go`) e `cmd/pull.go`.

0. **Pré-condição:** o runner recebe a lista **final** de repositórios, já descoberta e filtrada (Fase 9, etapa de descoberta). Ele não faz busca e conhece o total desde o início.
1. **Pool de workers** com a biblioteca padrão (sem dependência nova):
   - Um canal de jobs, `workers = min(max_concurrent, len(repos))` goroutines e `sync.WaitGroup` (`wg.Go`, disponível desde o Go 1.25).
   - Sequencial é só o caso `workers = 1` (quando `parallel_enabled=false`), com o mesmo caminho de código.
2. **Cada worker** processa um repositório:
   1. Se a branch atual não tem upstream (ou o HEAD está destacado), o pull não é executado e o status é `SKIPPED` com o motivo. Hoje o `git pull` falha nesse caso e conta como erro.
   2. Backup, se habilitado (repositórios diferentes, sem conflito entre workers).
   3. `git pull` com `exec.CommandContext` e timeout por repositório (`--git-timeout`, default 300s).
   4. `GetRepoBranches`.
   5. Devolve um `RepoResult{Name, Path, Branches, Status(OK|FAILED|TIMEOUT|SKIPPED), PullSummary, Output, Err, Duration}` por um canal de resultados.
3. **Saída do git capturada** (`CombinedOutput` em um buffer), em vez de ir direto ao terminal, para não misturar repositórios. Do texto capturado:
   - `PullSummary` guarda uma linha resumida: `Already up to date.`, `Fast-forward: 3 files changed, 10 insertions(+), 2 deletions(-)` ou a primeira linha `fatal:`/`error:`.
   - A saída completa aparece em `--log-level=debug` ou quando o repositório falha.
4. **Modo não interativo quando `workers > 1`**:
   - `GIT_TERMINAL_PROMPT=0`, para não pedir usuário/senha HTTPS.
   - `GIT_SSH_COMMAND="ssh -o BatchMode=yes"`, aplicado **apenas** se o usuário não tiver definido `GIT_SSH_COMMAND`.
   - Em falha de autenticação, a mensagem sugere rodar em modo sequencial (`--git-parallel-enabled=false`, que mantém o prompt interativo) ou configurar `ssh-agent`/credential helper.
   - No modo sequencial (`workers = 1`) o comportamento atual continua: o stdin fica ligado e o prompt de senha funciona.
5. **Uma única goroutine imprime** os resultados: ela lê o canal de resultados e imprime o bloco de cada repositório inteiro, sem intercalar. Os workers nunca escrevem no terminal.
6. **Ctrl+C**: `signal.NotifyContext` cancela o contexto, os `git pull` em andamento recebem SIGKILL via `CommandContext`, o resumo parcial é impresso e o processo sai com código 130.
7. **Erros**: os workers nunca chamam `Logger("fatal")`. Ao final, se houver falhas, o resumo é impresso e o processo sai com **código 1** (mantém a semântica atual de terminar com erro), sem o stack trace do `fatal`.
8. **Validação**: `max_concurrent` passa a ter `min=1,max=100` e `timeout` tem `min=1`.

## 7. Fase 4 - Saída por repositório e resumo final

**Arquivo novo:** `internal/output/output.go`. Ele cuida da apresentação (blocos, resumo, cores e símbolos) e deixa o pacote `git` só com a lógica.

- `output.PrintHeader(total, skipped, workers)`, `output.PrintRepoResult(i, total, result)` e `output.PrintSummary(results, duration)`.
- Usa a mesma decisão de cor do logger (`--color`).
- Com `--log-level=warning` ou `error`, os blocos de repositórios com sucesso são omitidos e aparecem só as falhas e o resumo. O resumo aparece sempre.

## 8. Exemplos de mensagens (antes x depois)

> Markdown não renderiza cores. As cores de cada elemento estão indicadas nos comentários `◀`.

### 8.1 Hoje (antes)

```text
2026-11-08 10:15:02 INFO Starting enhanced git repositories update. baseDir=/home/aecio/git parallel=true max_concurrent=15 backup_enabled=false backup_dir=./backups skip_repos=[old-project]
2026-11-08 10:15:02 INFO Repository filter configured. skip_count=1
2026-11-08 10:15:02 INFO Repository filter initialized. filter_stats=map[skip_count:1]
2026-11-08 10:15:02 INFO Update configuration prepared. parallel=true max_concurrent=15 timeout=30s backup=false filter_stats=map[skip_count:1]
2026-11-08 10:15:02 INFO Scanning for git repositories. baseDir=/home/aecio/git
2026-11-08 10:15:02 INFO Git repositories found. count=4
------------- BEGIN -------------
2026-11-08 10:15:02 INFO Updating repository. repository=updateGit path=/home/aecio/git/updateGit branch=main
[INFO] Updating repository: 'updateGit' on branch 'main'
If necessary, enter login/password when prompted.
2026-11-08 10:15:02 INFO Executing git pull. repository=/home/aecio/git/updateGit
Already up to date.
2026-11-08 10:15:03 INFO Git pull completed successfully. repository=/home/aecio/git/updateGit
---------------------------------
...
2026-11-08 10:15:09 ERROR <nil> error="Failed to update repository. repository=private-repo error=git pull failed ... (/home/aecio/git/updateGit/internal/git/git.go:222)" stack=[{"func":"Logger","line":"170","source":"common.go"},...]
2026-11-08 10:15:09 INFO Repository update completed. total=3 success=2 errors=1
2026-11-08 10:15:09 FATAL <nil> error="Update completed with 1 errors out of 3 repositories ..." stack=[...]
```

### 8.2 Depois - execução em paralelo (nível `info`, padrão)

```text
10:15:02 INFO  Scanning /home/aecio/git                                   ◀ rótulo ciano, timestamp cinza
10:15:02 INFO  Found 52 repositories · 2 skipped by filter · 50 to update
10:15:02 INFO  Updating in parallel with 15 workers (timeout 5m0s per repository)

✔ [1/50] updateGit                                            0.8s        ◀ ✔ e nome em verde
   ● main → origin/main · up to date                                      ◀ ● main em verde negrito; upstream em cinza
   ○ develop, feature/parallel-pull (ahead 1)                             ◀ "feature/parallel-pull (ahead 1)" em amarelo
   ↓ Already up to date.                                                  ◀ cinza

✔ [2/50] helm-charts                                          2.1s
   ● release/7.1 → origin/release/7.1 · up to date
   ○ main, gh-pages, fix/ingress-tls, +6 more
   ↓ Fast-forward: 4 files changed, 37 insertions(+), 5 deletions(-)

✘ [3/50] private-repo                                         0.4s        ◀ ✘ e nome em vermelho
   ● develop → origin/develop · behind 3
   ○ main
   ↓ fatal: could not read Username for 'https://github.com': terminal prompts disabled   ◀ vermelho
   ⚠ Authentication required. Run sequentially (--git-parallel-enabled=false)            ◀ amarelo
     or configure ssh-agent / a git credential helper.

⏱ [4/50] huge-monorepo                                        5m0s        ◀ ⏱ e nome em amarelo
   ● main → origin/main · behind 120
   ↓ Timeout after 5m0s. Increase it with --git-timeout.

✔ [5/50] lab-notes                                            0.3s
   ● (detached HEAD at 3f2a1c9)                                           ◀ amarelo: atenção
   ○ main, draft
   ↓ Already up to date.

⏭ [6/50] scratch                                              0.2s        ◀ ⏭ e nome em amarelo
   ● main · no upstream                                                   ◀ "no upstream" em amarelo
   ↓ Skipped pull: current branch has no upstream.

...

════════════════════════════ Summary ════════════════════════════
  Total 50   ✔ Updated 46   ✘ Failed 1   ⏱ Timeout 1   ⏭ Skipped 2   ⌛ 41.7s
  Failed:
    ✘ private-repo    authentication required
    ⏱ huge-monorepo   timeout after 5m0s
  Skipped (no upstream): scratch, tmp-experiment
  Filtered out (--skip-repos): old-project, broken-repo
═════════════════════════════════════════════════════════════════
```

A saída termina com código `1`, porque houve falhas.

### 8.3 Depois - sem cor (`--color=never` ou saída redirecionada para arquivo/pipe)

```text
10:15:02 INFO  Found 52 repositories, 2 skipped by filter, 50 to update
10:15:02 INFO  Updating in parallel with 15 workers (timeout 5m0s per repository)

[OK]   [1/50] updateGit (0.8s)
       * main -> origin/main (up to date)
       - others: develop, feature/parallel-pull (ahead 1)
       > Already up to date.

[FAIL] [3/50] private-repo (0.4s)
       * develop -> origin/develop (behind 3)
       - others: main
       > fatal: could not read Username for 'https://github.com': terminal prompts disabled
       ! Authentication required. Run sequentially (--git-parallel-enabled=false) or configure ssh-agent / a git credential helper.

==================== Summary ====================
Total 50 | Updated 46 | Failed 1 | Timeout 1 | Skipped 2 | 41.7s
Failed: private-repo (authentication required), huge-monorepo (timeout after 5m0s)
```

### 8.4 Depois - níveis de log

```text
10:15:02 DEBUG Command 'git' found in system PATH.                        ◀ tudo em cinza
10:15:02 INFO  Found 52 repositories · 2 skipped by filter · 50 to update  ◀ rótulo ciano
10:15:02 WARN  Backup directory ./git_backups does not exist, creating it  ◀ amarelo
10:15:09 ERROR Failed to create backup for private-repo: permission denied ◀ vermelho (sem stack trace)
10:15:09 FATAL Directory does not exist: /home/aecio/gti                   ◀ branco sobre fundo vermelho
```

Com `--log-level=debug` (ou `-D`), o timestamp é completo, ERROR/FATAL mostram `arquivo:linha` e cada bloco mostra a lista completa de branches e a saída inteira do `git pull`:

```text
2026-11-08 10:15:09 ERROR Failed to create backup for private-repo: permission denied (internal/git/git.go:212)

✘ [3/50] private-repo                                         0.4s
   ● develop → origin/develop · behind 3
   ○ main
   ┆ git pull output:
   ┆   fatal: could not read Username for 'https://github.com': terminal prompts disabled
```

### 8.5 Depois - filtrando por nível

```bash
# Mostra só avisos, erros, falhas e o resumo (bom para cron)
updateGit pull -G ~/git -J 20 --log-level=warning

# Tudo, inclusive debug
updateGit pull -G ~/git -J 20 -L debug
```

Saída de `--log-level=warning` sem falhas:

```text
════════════════════════════ Summary ════════════════════════════
  Total 50   ✔ Updated 48   ✘ Failed 0   ⏭ Skipped 2   ⌛ 38.2s
═════════════════════════════════════════════════════════════════
```

### 8.6 Depois - barra de progresso ao vivo (Fase 8)

Só em terminal interativo. Fica sempre na última linha, e os blocos dos repositórios que terminam são impressos acima dela:

```text
✔ [21/50] helm-charts                                         2.1s
   ● release/7.1 → origin/release/7.1 · up to date
   ↓ Fast-forward: 4 files changed, 37 insertions(+), 5 deletions(-)

⠼ [████████████░░░░░░░░░░░░] 21/50  42%  ✔ 19 ✘ 1 ⏱ 0 ⏭ 1 │ running 15: infra-live, docs-site, api +12 │ backup 2/4 │ 00:12 · ETA ~00:17
```

- `⠼` é um *spinner* animado; a barra fica verde e `✘` vermelho quando houver falhas.
- "running" mostra até 3 nomes e `+N`; "backup 2/4" mostra os backups em andamento e o limite (Fase 10).
- O ETA é estimado pela duração média dos repositórios já concluídos.
- Em uma saída que não é terminal (cron, pipe ou arquivo) a barra **não** aparece; o log em arquivo nunca recebe a barra.

### 8.7 Depois - log em arquivo (Fase 7)

```bash
updateGit pull -G ~/git -J 20 --log-file-enabled
```

No terminal, ao final:

```text
10:15:44 INFO  Log saved to /tmp/updateGit_2026-11-08_10-15-02.log
```

Conteúdo do arquivo: sem ANSI, timestamp completo e os mesmos blocos e resumo da tela, em texto puro:

```text
===== updateGit 0.2.0 · started 2026-11-08 10:15:02 · pid 48213 =====
2026-11-08 10:15:02 INFO  Found 52 repositories · 2 skipped by filter · 50 to update
2026-11-08 10:15:02 INFO  Updating in parallel with 20 workers (timeout 5m0s per repository)
[OK]   [1/50] updateGit (0.8s)
       * main -> origin/main (up to date)
       - others: develop, feature/parallel-pull (ahead 1)
       > Already up to date.
...
==================== Summary ====================
Total 50 | Updated 46 | Failed 1 | Timeout 1 | Skipped 2 | 41.7s
===== finished 2026-11-08 10:15:44 · exit code 1 =====
```

### 8.8 Depois - busca recursiva (Fase 9)

```bash
updateGit pull -G ~/git        # levels 1 and 2 (default)
updateGit pull -G ~/git -R 3   # levels 1, 2 and 3
updateGit pull -G ~/git -R 1   # only level 1 (previous behavior)
```

```text
⠹ Scanning /home/aecio/git (levels 1-2) · 14 directories · 61 repositories found...   ◀ spinner enquanto busca (só em TTY)
10:15:02 INFO  Scan finished in 0.3s: 87 repositories in 14 directories (levels 1-2)
10:15:02 INFO  Skipped by filter: 3 · Ignored directories: node_modules, vendor · To update: 84
10:15:02 INFO  Updating in parallel with 15 workers (timeout 5m0s per repository)

✔ [1/84] github/helm-charts                                     0.8s      ◀ caminho relativo ao base_dir
   ● main → origin/main · up to date
✔ [2/84] gitlab/terraform-modules                               1.4s
   ● main → origin/main · up to date
```

O nome exibido passa a ser o caminho relativo ao `base_dir`; para repositórios do primeiro nível, é igual ao nome atual da pasta.

Com `-R 3`, as linhas de log mostram `(levels 1-3)` e os nomes podem ter três partes (ex.: `github/org-x/api`).

Nenhum `git pull` começa antes da linha `Scan finished`: o total (`84`) usado em `[n/84]`, na barra de progresso, nos logs e no resumo é conhecido desde o início.

Exemplo de árvore e do que é encontrado:

```text
~/git                         ◀ base_dir
├── updateGit/  (.git)        ◀ nível 1: repositório → encontrado; a busca NÃO entra nele
├── github/                   ◀ nível 1: pasta comum → a busca entra
│   ├── helm-charts/ (.git)   ◀ nível 2: repositório → encontrado
│   └── org-x/                ◀ nível 2: pasta comum → só entra com -R 3
│       ├── api/ (.git)       ◀ nível 3: encontrado só com -R 3
│       └── team-y/           ◀ nível 3: pasta comum → nunca entra (máximo de 3 níveis)
│           └── web/ (.git)   ◀ nível 4: NUNCA encontrado
└── node_modules/             ◀ ignorado (exclude_dirs)
```

## 9. Configuração: flags, variáveis de ambiente e arquivo

Segue as convenções atuais: as chaves de config mapeiam para variáveis `CLI_` com `.`/`-` trocados por `_`, e a ligação é feita em `bindEnvs` no `cmd/root.go`.

| Flag | Curta | Chave no `.updateGit.yaml` | Variável de ambiente | Default | Status |
|---|---|---|---|---|---|
| `--git-parallel-enabled` | `-P` | `git.parallel_enabled` | `CLI_GIT_PARALLEL_ENABLED` | `true` | existe (passa a funcionar) |
| `--git-max-concurrent` | `-J` | `git.max_concurrent` | `CLI_GIT_MAX_CONCURRENT` | `10` | existe (passa a funcionar e ganha validação 1-100) |
| `--git-timeout` | `-T` | `git.timeout` | `CLI_GIT_TIMEOUT` | `300` (segundos) | **novo** |
| `--log-level` | `-L` | `log.level` | `CLI_LOG_LEVEL` | `info` | **novo** |
| `--color` | - | `log.color` | `CLI_LOG_COLOR` | `auto` | **novo** |
| `--debug` | `-D` | `debug` | `CLI_DEBUG` | `false` | existe (equivale a `--log-level=debug`) |
| `--log-file-enabled` | `-O` | `log.file.enabled` | `CLI_LOG_FILE_ENABLED` | `false` | **novo** (Fase 7) |
| `--log-file-dir` | - | `log.file.directory` | `CLI_LOG_FILE_DIRECTORY` | `/tmp` | **novo** (Fase 7) |
| `--log-file-level` | - | `log.file.level` | `CLI_LOG_FILE_LEVEL` | igual a `log.level` | **novo** (Fase 7) |
| `--progress` | - | `log.progress` | `CLI_LOG_PROGRESS` | `auto` | **novo** (Fase 8): `auto`, `always` ou `never` |
| `--git-max-depth` | `-R` | `git.max_depth` | `CLI_GIT_MAX_DEPTH` | `2` | **novo** (Fase 9): `1`, `2` ou `3` |
| `--git-exclude-dirs` | - | `git.exclude_dirs` | `CLI_GIT_EXCLUDE_DIRS` | `node_modules,vendor,.terraform,.venv` | **novo** (Fase 9) |
| `--backup-max-concurrent` | - | `backup.max_concurrent` | `CLI_BACKUP_MAX_CONCURRENT` | `4` | **novo** (Fase 10): 1 a 100 |

> O `-P` é booleano com default `true`; para desligar é preciso usar `--git-parallel-enabled=false`. Isso precisa ficar explícito na documentação. Como alternativa, `-J 1` também significa sequencial.

Exemplo de `.updateGit.yaml`:

```yaml
git:
  base_dir: "./git_repos"
  parallel_enabled: true
  max_concurrent: 15
  # Timeout (in seconds) for each "git pull"
  timeout: 300
  # Directory levels searched for repositories: 1 (only base_dir children), 2 (default) or 3 (maximum).
  # The search never goes into a directory that is already a git repository.
  max_depth: 2
  # Directory names ignored by the recursive search
  exclude_dirs:
    - "node_modules"
    - "vendor"
    - ".terraform"
    - ".venv"

backup:
  enabled: false
  directory: "./backups"
  strategy: "copy"
  # Maximum number of simultaneous backups (independent of git.max_concurrent)
  max_concurrent: 4

log:
  # Minimum log level: debug, info, warning, error
  level: "info"
  # Colored output: auto, always, never
  color: "auto"
  # Live progress bar: auto (only in a terminal), always, never
  progress: "auto"
  file:
    # Save the log to <directory>/updateGit_YYYY-MM-DD_HH-MM-SS.log
    enabled: false
    directory: "/tmp"
    # Empty = same as log.level
    level: ""
```

Mudanças no `internal/config/config.go`: novos campos `Git.Timeout` (`validate:"omitempty,min=1"`), `Git.MaxDepth` (`min=1,max=3`), `Git.ExcludeDirs`, `Backup.MaxConcurrent` (`min=1,max=100`), `Log.Level` / `Log.Color` / `Log.Progress` (`oneof`) e `Log.File.{Enabled,Directory,Level}`, defaults em `SetDefaultConfig`, e remoção da variável global `config.Timeout`, que não é usada.

## 10. Testes

Regras gerais:

- Biblioteca padrão, sem testify, seguindo o padrão atual: `setupGitEnv`, repositórios em `t.TempDir()` e remoto local `--bare`, sem acesso à rede.
- Tudo precisa passar em `make check`, **com `-race`**. Esse é o teste principal do paralelismo.
- Para testar a concorrência sem depender do git, o pacote `git` expõe uma variável `pullFunc` (troca de implementação só em teste). Para testar a saída, o logger e o renderer recebem um `io.Writer` injetável.
- **Cobertura:** nenhum pacote pode cair em relação à cobertura atual (config 100%, filter 100%, git 91%, common 83%, backup 81%), e o novo pacote `internal/output` deve ter ≥ 85% (`make coverage`).

### 10.1 Fase 1 - Logger

| Teste | Pacote | O que valida |
|---|---|---|
| `TestLoggerLevels` | `common` | Com `level=warning`, DEBUG e INFO não saem e WARNING/ERROR saem (writer em `bytes.Buffer`). Tabela com os 4 níveis. |
| `TestLoggerColorModes` | `common` | `never` não gera ANSI, `always` gera ANSI e `auto` gera ANSI só com TTY e sem `NO_COLOR`. |
| `TestLoggerLevelColors` | `common` | Cada nível usa o código ANSI da paleta (seção 4, item 3) e os rótulos têm largura fixa. |
| `TestLoggerNoStackTraceOutsideDebug` | `common` | ERROR em nível `info` não imprime `stack=`; em `debug` imprime `arquivo:linha`. |
| `TestLoggerBeforeInit` | `common` | `Logger` chamado antes do `InitLogger` usa o padrão (`info`, `auto`) sem *panic*. |
| `TestLoggerConcurrentUse` | `common` | 100 goroutines logando ao mesmo tempo, sem *data race* (`-race`) e sem linhas cortadas. |
| `TestDebugFlagOverridesLogLevel` | `cmd` | `-D` com `--log-level=error` resulta em nível `debug`. |

### 10.2 Fase 2 - Branches

| Teste | Pacote | O que valida |
|---|---|---|
| `TestGetRepoBranches` | `git` | Tabela de cenários: branch atual com upstream e em dia; `ahead N`; `behind N`; `ahead/behind` juntos; sem upstream; upstream `gone`; detached HEAD (SHA curto); várias branches em ordem alfabética; nome com `/` (`feature/x`). |
| `TestGetRepoBranchesNotARepo` | `git` | Diretório que não é repositório retorna erro tipado (`*GitError`). |

### 10.3 Fase 3 - Paralelismo

| Teste | Pacote | O que valida |
|---|---|---|
| `TestUpdateParallelManyRepos` | `git` | 50 repositórios clonados de um remoto local, `max_concurrent=15`: todos atualizados e sem *data race*. |
| `TestUpdateRespectsMaxConcurrent` | `git` | Com a `pullFunc` fake e um contador atômico, o pico de execuções simultâneas nunca passa de `max_concurrent` (15, 20 e 50). |
| `TestUpdateSequentialWhenDisabled` | `git` | `parallel_enabled=false` ou `max_concurrent=1` resulta em pico 1, com stdin ligado ao terminal. |
| `TestUpdateWorkersCappedByRepoCount` | `git` | `max_concurrent=50` com 3 repositórios cria só 3 workers. |
| `TestPullTimeout` | `git` | Uma `pullFunc` que bloqueia resulta em status `TIMEOUT` dentro do prazo, e os outros repositórios terminam. |
| `TestPullFailureDoesNotStopOthers` | `git` | Um repositório com remoto inválido fica `FAILED`, os outros ficam `OK` e o resultado agregado indica erro. |
| `TestSkipWhenNoUpstream` | `git` | Branch sem upstream e detached HEAD ficam `SKIPPED` com o motivo, e o `git pull` não é chamado. |
| `TestNonInteractiveEnvInParallel` | `git` | Com `workers > 1`: `GIT_TERMINAL_PROMPT=0` e `GIT_SSH_COMMAND` com `BatchMode=yes`, preservando o `GIT_SSH_COMMAND` do usuário quando já existir. |
| `TestCancelStopsPendingRepos` | `git` | O cancelamento do contexto (simula Ctrl+C) interrompe os pulls em andamento, não inicia os pendentes e devolve o resultado parcial. |
| `TestBackupInParallel` | `git` | Backups `copy` e `stash` com 20 repositórios em paralelo, sem *data race* e com um backup por repositório. |
| `TestSummarizePullOutput` | `git` | Extrai `Already up to date.`, a linha de *Fast-forward* com estatística e a primeira linha `fatal:`/`error:`. |
| `TestPullExitCode` | `cmd` | Com falhas, `runUpdate` devolve erro e o processo sai com código 1; sem falhas, com código 0. |

### 10.4 Fase 4 - Saída

| Teste | Pacote | O que valida |
|---|---|---|
| `TestRenderRepoResult` | `output` | *Golden files* (`testdata/*.golden`) dos blocos OK, FAILED, TIMEOUT, SKIPPED, detached e `+N more`, com e sem cor. Para atualizar: `go test ./internal/output -update`. |
| `TestRenderSummary` | `output` | Contagens, lista de falhas em ordem alfabética, "Skipped (no upstream)" e "Filtered out". |
| `TestRenderHidesSuccessAtWarningLevel` | `output` | Com `warning`/`error`, só falhas e o resumo aparecem; o resumo aparece sempre. |
| `TestRenderSymbolsFallback` | `output` | Sem cor, troca `✔ ✘ ● ⏭` por `[OK] [FAIL] * [SKIP]` e não gera ANSI. |
| `TestRenderBranchTruncation` | `output` | Mais de 8 branches mostra `+N more`; em `debug` mostra a lista completa. |

### 10.5 Configuração e flags

| Teste | Pacote | O que valida |
|---|---|---|
| `TestSetDefaultConfig` (atualizado) | `config` | Novos defaults: `git.timeout=300`, `log.level=info`, `log.color=auto`. |
| `TestConfigValidation` | `config` | Rejeita `max_concurrent` 0 e 101, `timeout` 0, `log.level=verbose` e `log.color=rainbow`; aceita os limites 1 e 100. |
| `TestCLIVersionIsSemver` | `config` | `CLIVersion` segue a regex semver **sem** `v`, a mesma usada no CI. |
| `TestConfigPrecedence` | `cmd` | Para `log.level`, `git.timeout` e `git.max_concurrent`: flag > variável `CLI_*` > arquivo > default. Exige extrair do `initConfig` uma função `loadConfig(v *viper.Viper, flags)` testável. Serão os primeiros testes do pacote `cmd`. |

### 10.6 Fase 5 - Release e CI

| Teste/verificação | Onde | O que valida |
|---|---|---|
| `TestParseChecksumSha256sumFormat` | `update` | `ParseChecksum` lê a saída real do `sha256sum bin/*` (dois espaços e prefixo `bin/`). |
| `TestReleaseAssetNames` | `update` | O nome esperado por `ApplyUpdate` (`updateGit-<os>-<arch>`) bate com a lista de plataformas suportadas. Extrair `assetName()` para uma função testável. |
| `TestLongVersionShowsBuildInfo` | `getinfo` | `-V` mostra `CLICommit` e `CLIBuildDate` quando injetados e `unknown` quando não. |
| `make verify-artifacts` | Makefile + CI | Novo target usado pelo CI e localmente: confere os 4 binários, o SBOM, o `sha256sum -c` e se `updateGit -v` é igual à versão. |
| `make lint-workflows` | Makefile | Novo target: roda `actionlint` (com `shellcheck`) em `.github/workflows/`. Entra no `make check` quando o `actionlint` estiver instalado (avisa e pula se não estiver). |
| Pré-release `0.2.0-rc.1` | GitHub | Teste ponta a ponta descrito na seção 11.6. |

### 10.7 Testes existentes que mudam

- `internal/git/git_test.go`:
  - `TestUpdateRepositoriesWithConfig` deve verificar o novo resultado agregado (`[]RepoResult`/resumo) em vez de só `err == nil`.
  - No cenário com repositório sem upstream, esperar `SKIPPED` em vez de falha.
  - `TestPullRepository` passa a usar a versão com contexto e saída capturada.
- `internal/common/common_test.go`: `TestLoggerNonFatalLevels` é substituído pelos testes da seção 10.1.
- `internal/config/config_test.go`: `TestSetDefaultConfig` e `TestBackupStrategyValidation` ganham os novos campos.
- `internal/git/git_test.go`: `TestIsGitRepository` e `TestFindRepositories` cobrem `.git` como arquivo e o novo `max_depth` (com `1`, o resultado tem que ser igual ao atual; o default passa a ser `2`).

### 10.8 Fase 6 - Checagens de desenvolvimento

| Teste/verificação | O que valida |
|---|---|
| `make check` em CI de PR | Todo PR roda o mesmo `make check` do desenvolvedor; o PR não pode ser mergeado se falhar (*branch protection*). |
| Erro proposital (validação manual da fase) | Introduzir em uma branch de teste: arquivo sem gofmt, `err` não tratado, `go.mod` sem `tidy` e workflow YAML inválido. Cada um precisa ser barrado pelo target correspondente e pelo hook de pre-commit. |
| `make coverage-check` | Falha se a cobertura total ficar abaixo do mínimo (`COVERAGE_MIN`, sugerido 70%). |

### 10.9 Fase 7 - Log em arquivo

| Teste | Pacote | O que valida |
|---|---|---|
| `TestLogFileName` | `common` | Com relógio injetado, gera `updateGit_2026-11-08_10-15-02.log` (ou `_10.log`, conforme a decisão 3.9) no diretório configurado. |
| `TestLogFileNoANSI` | `common` | Com `--color=always` na tela, o arquivo não contém sequências ANSI. |
| `TestLogFileLevelIndependent` | `common` | `log.level=warning` e `log.file.level=debug`: a tela mostra só WARNING+ e o arquivo tem DEBUG+. |
| `TestLogFilePermissions` | `common` | O arquivo é criado com permissão `0600`. |
| `TestLogFileDirCreatedOrError` | `common` | Diretório inexistente é criado; diretório sem permissão de escrita gera erro claro **antes** de iniciar os pulls. |
| `TestLogFileContainsBlocksAndSummary` | `output` | Os blocos e o resumo também vão para o arquivo, em texto puro, com cabeçalho e rodapé de execução. |
| `TestLogFileAppendSameHour` | `common` | Só se a decisão 3.9 for a alternativa (só a hora): duas execuções no mesmo arquivo, separadas por cabeçalho. |

### 10.10 Fase 8 - Barra de progresso

| Teste | Pacote | O que valida |
|---|---|---|
| `TestProgressRender` | `output` | *Golden files* da linha da barra em 0%, 42% e 100%, com falhas, com `+N` em "running" e com larguras de terminal de 60, 80 e 200 colunas (truncamento sem quebrar a linha). |
| `TestProgressETA` | `output` | ETA calculado pela média, e `--:--` antes do primeiro resultado. |
| `TestProgressModes` | `output` | `auto` desliga quando a saída não é TTY; `never` nunca desenha; `always` desenha mesmo sem TTY. |
| `TestProgressDisabledWhenSequentialInteractive` | `output` | Com `workers = 1` (prompt de senha possível), a barra fica desligada. |
| `TestProgressNotInLogFile` | `output` | O arquivo de log nunca recebe a barra nem sequências de controle (`\r`, `\033[2K`). |
| `TestTerminalWriterInterleaving` | `output` | Logs e blocos escritos por várias goroutines limpam a barra, escrevem a linha inteira e redesenham a barra, sem *data race*. |

### 10.11 Fase 9 - Busca recursiva

| Teste | Pacote | O que valida |
|---|---|---|
| `TestFindRepositoriesDepth` | `git` | Árvore com repositórios nos níveis 1, 2, 3 e 4: `max_depth=1` encontra só o nível 1; `max_depth=2` (default) encontra os níveis 1 e 2; `max_depth=3` encontra os níveis 1, 2 e 3; o nível 4 **nunca** é encontrado. |
| `TestFindRepositoriesDoesNotWalkInsideRepo` | `git` | Um repositório no nível 1 com subpastas e um repositório aninhado: a busca não lê nada dentro dele (verificado contando os diretórios visitados). |
| `TestDiscoveryCompletesBeforeUpdate` | `git` | Com uma `pullFunc` fake que registra o instante de cada chamada e uma busca instrumentada: nenhum pull começa antes de a descoberta terminar, e o total informado ao renderer/barra é igual à quantidade de repositórios encontrados menos os filtrados. |
| `TestDiscoveryResultSummary` | `git` | O resultado da descoberta traz repositórios, diretórios visitados, diretórios ignorados e avisos (ex.: permissão negada), usados nos logs. |
| `TestMaxDepthValidation` | `config` | `max_depth` 0 e 4 são rejeitados; 1, 2 e 3 são aceitos; o default é 2. |
| `TestFindRepositoriesExcludeDirs` | `git` | Repositórios dentro de `node_modules`/`vendor` não são encontrados. |
| `TestFindRepositoriesSymlinkLoop` | `git` | Um link simbólico apontando para um diretório pai não causa loop (links não são seguidos). |
| `TestFindRepositoriesWorktree` | `git` | `git worktree add` (`.git` como arquivo) é reconhecido como repositório. |
| `TestRepositoryRelativeName` | `git` | O nome é o caminho relativo ao `base_dir` (ex.: `github/repo`), igual ao atual com profundidade 1. |
| `TestSkipReposMatchesRelativePath` | `filter` | `skip_repos` aceita o nome da pasta (`repo`) e o caminho relativo (`github/repo`). |
| `TestCopyBackupNestedName` | `backup` | Dois repositórios com o mesmo nome de pasta em diretórios diferentes geram backups em caminhos distintos. |

### 10.12 Fase 10 - Concorrência de backups

| Teste | Pacote | O que valida |
|---|---|---|
| `TestBackupMaxConcurrent` | `git` | Com `git.max_concurrent=20` e `backup.max_concurrent=3`, usando backup fake com contador atômico: o pico de backups é 3 e o de pulls é 20. |
| `TestBackupLimitDoesNotBlockWithoutBackup` | `git` | Com backup desabilitado, o limite não interfere no pull. |
| `TestBackupLimitCappedByWorkers` | `git` | `backup.max_concurrent` maior que `git.max_concurrent` é reduzido ao número de workers. |
| `TestBackupWaitCancel` | `git` | Ctrl+C enquanto um worker espera vaga de backup cancela sem *deadlock*. |
| `TestBackupMaxConcurrentValidation` | `config` | 0 e 101 são rejeitados. |

Teste manual de carga, com o resultado registrado no PR: rodar `updateGit pull -G <dir com 50+ repos>` com `-J 1`, `-J 15` e `-J 50` e comparar os tempos.

## 11. Fase 5 - CI de release no GitHub Actions

**Arquivo novo:** `.github/workflows/release.yml`. Ajustes em `Makefile`, `CONTRIBUTING.md` e `internal/config/config.go`.

### 11.1 Gatilho e regras

- Roda **somente** em `push` de tag no formato de versão semântica (`MAJOR.MINOR.PATCH`, opcionalmente com `-pré-release`), sem prefixo `v`. Exemplos: `0.2.0`, `1.0.0` e `0.3.0-rc.1`.
- O filtro de tags do GitHub (glob) faz uma primeira seleção, e um passo valida a tag com a regex oficial do [semver.org](https://semver.org/#is-there-a-suggested-regular-expression-regex-to-check-a-semver-string) (sem *build metadata*).
- **A tag tem que ser igual a `config.CLIVersion`** (`go run . -v`). Se for diferente, o workflow falha antes de compilar, o que evita publicar binários que se anunciam com outra versão e quebrariam o `updateGit update`.
- Antes de compilar, roda `make check` (gofmt, vet e testes com `-race`). Se algum teste falhar, nada é publicado.

### 11.2 Binários e artefatos publicados

Plataformas suportadas (as mesmas do `Makefile` e do `internal/getinfo`, sem Windows):

| GOOS | GOARCH | Asset na release |
|---|---|---|
| linux | amd64 | `updateGit-linux-amd64` |
| linux | arm64 | `updateGit-linux-arm64` |
| darwin | amd64 | `updateGit-darwin-amd64` |
| darwin | arm64 | `updateGit-darwin-arm64` |

Além deles, a release recebe:

- `checksums.txt`: SHA256 de todos os arquivos, no formato `<hash>  bin/<arquivo>`. O prefixo `bin/` é **obrigatório**, porque o `ApplyUpdate` procura `config.CLICheckSumBinDir + assetName`.
- `updateGit.sbom.json`: SBOM CycloneDX gerado pelo Trivy.

> Os nomes dos assets e o formato do `checksums.txt` são um **contrato** com o `updateGit update` das versões já instaladas. Não podem mudar sem uma estratégia de compatibilidade.

O build continua sendo feito pelo `make build`, o mesmo usado localmente, com `CGO_ENABLED=0` e compilação cruzada a partir de um único runner `ubuntu-latest`. Go e Trivy são instalados pelo `jdx/mise-action` com as versões do `mise.toml`, as mesmas do ambiente de desenvolvimento.

### 11.3 Release

- **Título:** igual ao nome da tag (ex.: `0.2.0`).
- **Mensagem (notes):** fixa:

  ```markdown
  See changes in [CHANGELOG.md](https://github.com/aeciopires/updateGit/blob/main/CHANGELOG.md) file
  ```

- Tag com sufixo de pré-release (`-rc.1`, `-beta.2`...) é publicada com `--prerelease`. As demais viram a release *latest*.
- A release é criada com o `gh` CLI, que já vem no runner, em vez de uma action de terceiros. Se o job for executado de novo para a mesma tag, os assets são substituídos (`gh release upload --clobber`) e não é criada uma release duplicada.

### 11.4 Rascunho do workflow

```yaml
name: Release

on:
  push:
    tags:
      - '[0-9]+.[0-9]+.[0-9]+'     # 0.2.0
      - '[0-9]+.[0-9]+.[0-9]+-*'   # 0.2.0-rc.1 (pre-release)

permissions:
  contents: write   # create the release and upload assets

concurrency:
  group: release-${{ github.ref_name }}
  cancel-in-progress: false

jobs:
  release:
    name: Build and publish ${{ github.ref_name }}
    runs-on: ubuntu-latest
    timeout-minutes: 20
    env:
      TAG: ${{ github.ref_name }}
    steps:
      # Actions pinned by commit SHA (comment shows the version)
      - name: Checkout
        uses: actions/checkout@<SHA> # v4

      - name: Install Go and Trivy (versions from mise.toml)
        uses: jdx/mise-action@<SHA> # v2

      - name: Validate tag (semantic version) and CLIVersion
        run: |
          semver='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-((0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)(\.(0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*))*))?$'
          if ! echo "$TAG" | grep -Eq "$semver"; then
            echo "::error::Tag '$TAG' is not a semantic version (MAJOR.MINOR.PATCH, without 'v' prefix)"
            exit 1
          fi
          cli_version="$(go run . -v)"
          if [ "$TAG" != "$cli_version" ]; then
            echo "::error::Tag '$TAG' differs from config.CLIVersion '$cli_version' (internal/config/config.go)"
            exit 1
          fi

      - name: Check code (gofmt, vet, tests)
        run: make check

      - name: Build binaries, SBOM and checksums
        run: make build

      - name: Verify artifacts
        run: |
          for platform in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64; do
            test -s "bin/updateGit-$platform" || { echo "::error::Missing bin/updateGit-$platform"; exit 1; }
          done
          test -s bin/updateGit.sbom.json
          sha256sum -c bin/checksums.txt
          ./bin/updateGit-linux-amd64 -v | grep -qx "$TAG"

      - name: Create GitHub release
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          notes='See changes in [CHANGELOG.md](https://github.com/aeciopires/updateGit/blob/main/CHANGELOG.md) file'
          prerelease=""
          case "$TAG" in *-*) prerelease="--prerelease" ;; esac

          if gh release view "$TAG" >/dev/null 2>&1; then
            echo "Release $TAG already exists, replacing assets"
            gh release upload "$TAG" bin/* --clobber
          else
            gh release create "$TAG" bin/* \
              --title "$TAG" \
              --notes "$notes" \
              --verify-tag \
              $prerelease
          fi
```

> Na implementação, o conteúdo do passo "Verify artifacts" vira o target `make verify-artifacts` (seção 10.6), e o workflow passa a chamar `make verify-artifacts`. Assim a mesma verificação roda localmente e no CI.

> As actions ficam fixadas por SHA de commit (com a versão em comentário), para evitar ataque de cadeia de suprimentos via tag mutável. Os SHAs são preenchidos na implementação, e o Dependabot (`.github/dependabot.yml`, ecossistema `github-actions`) pode mantê-los atualizados.

### 11.5 Ajustes de apoio

- **`Makefile`:**
  - Os `LDFLAGS` passam a injetar `-X github.com/aeciopires/updateGit/internal/config.CLICommit=$(COMMIT)` e `...config.CLIBuildDate=$(DATE)` (decisão 3.8). O `updateGit -V` passa a exibir commit e data do build.
  - Remover a variável `GOLANG_SUPPORTED_PLATFORMS`, que não é usada, ou usá-la para gerar os builds em loop.
  - O `make build` deixa de depender de arquivos antigos em `bin/` (`rm -rf bin` no início), para que o `checksums.txt` nunca inclua sobras de builds anteriores.
- **`CONTRIBUTING.md`:** o fluxo de release passa a ser:
  1. Atualizar `CHANGELOG.md` e `config.CLIVersion` em um PR para a `main`.
  2. Depois do merge, criar e enviar a tag:

     ```bash
     git checkout main && git pull
     export CLI_VERSION="$(go run . -v)"
     git tag -a "$CLI_VERSION" -m "Release $CLI_VERSION"
     git push origin "$CLI_VERSION"
     ```

  3. Acompanhar o workflow **Release** na aba *Actions*. O passo manual "Create a release in Github and add the artifacts" sai do documento.
- **Proteção de tags (configuração no GitHub, fora do código):** criar um *ruleset* de tags que restringe quem pode criar, alterar ou apagar tags no padrão `*.*.*`.

### 11.6 Testes do CI

- Validar o YAML com `actionlint` (disponível via mise), localmente e no `make check` se o comando existir.
- Testar o fluxo completo em um fork (ou com uma pré-release `0.2.0-rc.1`):
  - A release é criada com título `0.2.0-rc.1`, marcada como *pre-release*, com a mensagem padrão e os 6 assets.
  - Uma tag inválida (`v0.2.0` ou `0.2`) não dispara o workflow ou falha na validação.
  - Uma tag diferente do `CLIVersion` falha antes do build.
  - Executar o job de novo substitui os assets sem duplicar a release.
- Depois da release final, rodar `updateGit update` a partir da versão anterior instalada e confirmar que o download e a verificação de checksum funcionam.

## 12. Fase 6 - Checagens de desenvolvimento no Makefile e CI de PR

Objetivo: rodar **todos** os testes e checagens com um comando, antes do commit e em cada PR, para pegar erros enquanto o código está sendo escrito.

### 12.1 Ferramentas (fixadas no `mise.toml`)

| Ferramenta | Uso |
|---|---|
| `golangci-lint` | Linter agregador. Pega erro não tratado, código morto, *shadowing*, problemas de segurança (`gosec`), erros de digitação e mais. |
| `govulncheck` (`go:golang.org/x/vuln/cmd/govulncheck`) | Vulnerabilidades conhecidas **no código efetivamente chamado** (Go vuln DB). |
| `trivy` (já existe) | Vulnerabilidades em dependências, segredos e configurações (`trivy fs`). |
| `actionlint` + `shellcheck` | Lint dos workflows do GitHub Actions e dos scripts dentro deles. |
| `pre-commit` | Executa as checagens automaticamente em `git commit` e `git push`. |

O `make check-deps` e o `make install-deps` passam a conhecer essas ferramentas: `go`, `git` e `trivy` continuam obrigatórios; as de desenvolvimento aparecem como `[MISSING]` com a dica `make install-deps`.

### 12.2 Configuração do linter (`.golangci.yml`)

- Base: `govet`, `staticcheck`, `errcheck`, `ineffassign` e `unused` (padrão do golangci-lint).
- Extras: `gosec`, `misspell`, `revive`, `errorlint` (uso correto de `%w`/`errors.Is`), `bodyclose` (`resp.Body` do `net/http`), `gocritic`, `unconvert` e `copyloopvar`.
- `goprintffuncname` + `govet printf` configurados para tratar o `common.Logger` como função printf. Assim, o bug corrigido na `0.1.1` (`%w` no `Logger`) é barrado automaticamente.
- Exclusões explícitas e comentadas, nunca globais. Os problemas que já existem no código atual são corrigidos nesta fase, não ignorados.

### 12.3 Novos targets do Makefile

| Target | O que faz | Rede? |
|---|---|---|
| `make lint` | `golangci-lint run ./...` | não |
| `make lint-fix` | `golangci-lint run --fix ./...` (corrige o que for automático) | não |
| `make mod-check` | `go mod tidy -diff` (falha se o `go.mod`/`go.sum` não estiver arrumado) e `go mod verify` | sim (cache) |
| `make test-short` | `go test -short ./...`: só testes rápidos, sem os de carga (50 repositórios) | não |
| `make coverage-check` | Roda os testes e falha se a cobertura total ficar abaixo de `COVERAGE_MIN` (padrão 70) | não |
| `make vuln` | `govulncheck ./...` | sim |
| `make security` | `trivy fs --scanners vuln,secret,misconfig --exit-code 1 --severity HIGH,CRITICAL .` | sim |
| `make lint-workflows` | `actionlint` nos workflows (Fase 5) | não |
| `make check` | **Rápido, para o dia a dia:** `fmt-check`, `mod-check`, `vet`, `lint`, `test` (com `-race`) | só cache |
| `make check-all` | **Completo, usado no CI e no pre-push:** `check` + `coverage-check` + `vuln` + `security` + `lint-workflows` + `build` + `verify-artifacts` | sim |
| `make install-hooks` | `pre-commit install --hook-type pre-commit --hook-type pre-push` | não |

Regras:

- Cada target imprime o que está checando e termina com `[OK]` ou `[ERROR]`. Em caso de erro, mostra a dica de correção (ex.: `Run 'make fmt'`, `Run 'go mod tidy'`, `Run 'make lint-fix'`).
- Os testes de carga (50 repositórios) usam `if testing.Short() { t.Skip() }`, para que o `make test-short` caiba no pre-commit.
- Todos os targets ganham a descrição `## ...` e aparecem no `make help`, na seção `##@ Tests and code quality`.

### 12.4 Hooks de git (`.pre-commit-config.yaml`)

Hooks locais (`repo: local`) que chamam o Makefile, para que a regra seja uma só:

| Estágio | Hooks | Tempo esperado |
|---|---|---|
| `pre-commit` | `make fmt-check`, `make lint`, `make test-short`, além de `check-yaml`, `end-of-file-fixer`, `trailing-whitespace` e `check-merge-conflict` | poucos segundos |
| `pre-push` | `make check-all` | ~1-2 min |

Instalação documentada no CONTRIBUTING: `make install-deps && make install-hooks`.

### 12.5 CI de pull request (`.github/workflows/ci.yml`)

- Gatilhos: `pull_request` e `push` na `main`.
- Matriz: `ubuntu-latest` e `macos-latest` (as duas plataformas suportadas).
- Passos: checkout, `jdx/mise-action` (mesmas versões do `mise.toml`) e `make check-all`. No macOS roda só `make check`, porque o build cruzado e o trivy já rodam no Linux.
- Cache de módulos Go e do banco do trivy.
- Upload do `coverage.html` como artefato do job.
- *Branch protection* na `main`: o job `ci` é obrigatório para o merge.
- Actions fixadas por SHA e `permissions: contents: read`.
- O workflow de release (Fase 5) reaproveita o `make check-all`.

## 13. Fase 7 - Exportar o log para arquivo

**Arquivos:** `internal/common/common.go` (ou `internal/common/logfile.go`), `internal/output/output.go`, `internal/config/config.go`, `cmd/root.go`, `cmd/pull.go`.

1. **Ativação:** `--log-file-enabled`/`-O` (default `false`). Diretório em `--log-file-dir` (default **`/tmp`**).
2. **Nome do arquivo:** `updateGit_<ANO>-<MES>-<DIA>_<HORA>-<MIN>-<SEG>.log`, ex.: `/tmp/updateGit_2026-11-08_10-15-02.log` (decisão 3.9). O horário é o **início** da execução, no fuso local.
3. **Criação:**
   - O diretório é criado se não existir (`0755`); o arquivo é criado com `0600`, porque o `/tmp` é compartilhado e o log contém caminhos e URLs de repositórios.
   - Se não for possível criar o arquivo, o programa falha **antes** de iniciar os pulls, com mensagem clara (`Cannot write log file /tmp/...: permission denied`).
   - O `/tmp` é usado literalmente (como pedido), inclusive no macOS, em vez de `os.TempDir()`, que lá aponta para `/var/folders/...`.
4. **Conteúdo:**
   - Sem cores ou sequências de controle, com timestamp completo e o nível de `log.file.level` (padrão: igual a `log.level`).
   - Cabeçalho (versão, início, pid, configuração efetiva sem segredos) e rodapé (fim, duração e código de saída).
   - Os blocos dos repositórios e o resumo também são gravados, em texto puro (mesmo formato da seção 8.3). A barra de progresso nunca vai para o arquivo.
5. **Implementação:**
   - O `InitLogger` recebe dois destinos: o console (`zerolog.ConsoleWriter` com cor) e o arquivo (`ConsoleWriter` com `NoColor: true`), combinados com `zerolog.MultiLevelWriter`, cada um com seu nível mínimo.
   - O renderer da Fase 4 recebe dois writers (tela e arquivo) e renderiza cada bloco duas vezes: com e sem cor.
   - O arquivo é fechado com `defer` no `Execute`, inclusive em `fatal` e em Ctrl+C (o `Logger("fatal")` passa a fechar o arquivo antes do `os.Exit`).
6. **Ao final:** `INFO Log saved to /tmp/updateGit_2026-11-08_10-15-02.log`.
7. Não há rotação nem limpeza automática: o `/tmp` é limpo pelo sistema (documentar). Uma opção de retenção fica para o futuro.

## 14. Fase 8 - Barra de progresso ao vivo

**Arquivos:** `internal/output/progress.go`, `internal/output/terminal.go`, `cmd/pull.go`. Nova dependência: `golang.org/x/term` (largura do terminal; módulo oficial do Go).

1. **Quando aparece:** `--progress=auto` (padrão) mostra a barra só se o stdout é um terminal **e** `workers > 1`. No modo sequencial a barra é desligada, porque o git pode pedir senha no mesmo terminal. `always` força e `never` desliga.
2. **Início:** a barra só aparece depois que a descoberta (Fase 9) termina, com o total já conhecido. Durante a descoberta aparece apenas o *spinner* de busca.
3. **Conteúdo** (seção 8.6): *spinner*, barra, `concluídos/total`, porcentagem, contadores por status, repositórios em andamento (até 3 nomes + `+N`), backups em andamento/limite (Fase 10), tempo decorrido e ETA.
4. **Terminal compartilhado:** um `TerminalWriter` com mutex é o **único** caminho para o stdout. Ele é usado pelo logger, pelo renderer e pela barra. Para cada linha de log ou bloco, ele:
   1. apaga a linha da barra (`\r\033[2K`);
   2. escreve o conteúdo completo;
   3. redesenha a barra.
5. **Atualização:** a goroutine de saída (Fase 4) usa `select` entre o canal de resultados e um `time.Ticker` de 100 ms. Só ela desenha a barra; os workers apenas enviam eventos (`started`, `backup_started`, `finished`) por canal.
6. **Largura:** usa a largura do terminal (`term.GetSize`), recalculada a cada desenho (redimensionamento). A linha é truncada para nunca quebrar, e o fallback é 80 colunas.
7. **Fim:** a barra é apagada antes do resumo final, e em Ctrl+C também, para não deixar lixo na tela.
8. Com `--color=never` a barra usa só ASCII (`[#########.........]`).

## 15. Fase 9 - Busca recursiva de repositórios

**Arquivos:** `internal/git/git.go` (`FindRepositories`), `internal/filter/filter.go`, `internal/backup/backup.go`, `internal/config/config.go`.

1. **Duas etapas, nessa ordem:**
   1. **Descoberta:** a busca percorre a árvore inteira (até o limite de níveis) e monta a lista completa de repositórios **antes** de qualquer atualização. Em seguida o filtro (`skip_repos`) é aplicado. Nenhum backup ou `git pull` começa nesta etapa.
   2. **Atualização:** só começa com a lista final pronta. O total `N` já é conhecido e é usado no contador `[n/N]`, na barra de progresso (porcentagem e ETA), nos logs (`To update: N`), no cabeçalho do log em arquivo e no resumo.
   - A função passa a devolver um `DiscoveryResult{Repos, VisitedDirs, IgnoredDirs, Warnings, Duration}`, usado para os logs da descoberta (seção 8.8).
   - Durante a descoberta, em terminal, aparece só um *spinner* com a contagem parcial (`Scanning... 14 directories · 61 repositories found`), porque o total ainda não existe. A barra de progresso da Fase 8 só começa na etapa de atualização.
   - Se a descoberta não encontrar nenhum repositório, o programa termina com WARNING e código 0, sem iniciar workers.
2. **Profundidade de 1 a 3 níveis:** `--git-max-depth`/`-R` aceita `1`, `2` ou `3` (default `2`, decisão 3.11). Nível 1 = filhos diretos do `base_dir`; nível 2 = filhos desses filhos; nível 3 = netos dos filhos. A busca **nunca** passa do terceiro nível.
3. **Algoritmo:** função recursiva simples `scan(dir, level)` com `os.ReadDir`. Ela começa no `base_dir` (nível 1) e só desce para um subdiretório **que não seja repositório** enquanto `level < max_depth`. Não é preciso um *walk* genérico, porque o limite é de no máximo 3 níveis:
   - **Diretório que é repositório git é registrado e a busca não entra nele**, em nenhum nível. Assim ela nunca percorre as subpastas de um repositório (código, `node_modules` do projeto, submódulos e repositórios aninhados), o que deixa a busca rápida e evita encontrar "repositórios dentro de repositórios".
   - Diretórios em `--git-exclude-dirs` (default `node_modules`, `vendor`, `.terraform`, `.venv`) e diretórios ocultos (exceto o próprio `base_dir`) são pulados.
   - Links simbólicos para diretórios **não** são seguidos (checagem com `DirEntry.Type()`), o que evita loops e buscas fora do `base_dir`.
   - Erro de permissão em um subdiretório gera WARNING e a busca continua. Hoje, um erro de leitura gera `fatal`.
4. **Detecção de repositório:** o `IsGitRepository` passa a aceitar `.git` como **diretório ou arquivo** (`gitdir: ...`), cobrindo *worktrees* e submódulos clonados à parte.
5. **Identificação:**
   - `Repository.Name` vira o **caminho relativo** ao `base_dir` (`github/repo`). Com profundidade 1 é igual ao atual.
   - `skip_repos` aceita o nome da pasta **ou** o caminho relativo (os dois casos são documentados).
   - O backup `copy` usa o caminho relativo como destino (`<backup_dir>/<timestamp>/github/repo`), evitando colisão entre repositórios homônimos.
6. **Saída:** `Scan finished in 0.3s: 87 repositories in 14 directories (levels 1-2)` (ou `1-3` com `-R 3`), seguido da linha com filtrados, ignorados e total a atualizar. Com `debug` aparece a lista de diretórios percorridos, ignorados e de repositórios em que a busca não entrou.
7. **Ordem:** a lista de repositórios é ordenada pelo caminho relativo, para que a ordem de início seja estável entre execuções.

## 16. Fase 10 - Limite separado de concorrência para backups

**Arquivos:** `internal/git/git.go` (runner), `internal/config/config.go`, `cmd/root.go`, `cmd/pull.go`.

1. **Opção:** `--backup-max-concurrent` (`backup.max_concurrent`, default `4`, validação `1-100`). O valor efetivo é `min(backup.max_concurrent, workers)`.
2. **Implementação:** um semáforo (canal com buffer) compartilhado pelos workers, adquirido só em volta do `CreateBackup`:

   ```go
   select {
   case backupSem <- struct{}{}:     // espera uma vaga
   case <-ctx.Done():                // Ctrl+C enquanto espera
       return result.Canceled()
   }
   info, err := bm.CreateBackup(repo.Path, repo.Name)
   <-backupSem                       // libera a vaga
   ```

3. Vale para as duas estratégias (`copy` e `stash`): simplicidade e previsibilidade. Com `stash`, um valor alto (ex.: igual a `max_concurrent`) é seguro e pode ser documentado como dica.
4. **Visibilidade:** o worker reporta `waiting backup slot` e `backup_started` para a barra de progresso (`backup 2/4`). Com `debug`, o tempo de espera por vaga aparece no bloco do repositório.
5. O backup de um repositório continua acontecendo **antes** do pull desse mesmo repositório, e um backup que falha continua impedindo o pull dele (comportamento atual: o erro é registrado e o pull segue). O plano mantém isso e só documenta.

## 17. Documentação, CHANGELOG e CLAUDE.md

Cada fase atualiza os arquivos abaixo no mesmo PR (veja a definição de pronto na [seção 1](#1-objetivo)).

### 17.1 O que muda em cada arquivo

| Arquivo | Fase | Conteúdo |
|---|---|---|
| `README.md` | 1 | Seção nova **"Log levels and colors"**: `--log-level`/`-L`, `--color`, `NO_COLOR`, relação com o `-D` e exemplo de saída por nível (seção 8.4). |
| `README.md` | 2, 4 | Seção **"Output"**: como ler o bloco de cada repositório (`●` branch atual, `○` outras, `↓` resultado do pull), estados `ahead/behind/no upstream/detached`, resumo final e códigos de saída (0, 1 e 130). |
| `README.md` | 3 | **Features**: paralelismo real. **Usage**: exemplos com `-J 15/20/50`, `-T`, `--git-parallel-enabled=false`. Seção nova **"Parallel mode and authentication"**: modo não interativo, ssh-agent, credential helper, `ControlMaster` e quando usar o modo sequencial. |
| `README.md` | 1, 3 | Exemplo de `.updateGit.yaml` e lista de variáveis de ambiente (`CLI_GIT_TIMEOUT`, `CLI_LOG_LEVEL`, `CLI_LOG_COLOR`). |
| `README.md` | 5 | **"Downloading CLI"**: link da página de releases, download por plataforma e verificação com `sha256sum -c checksums.txt`. **"Development Commands"**: novos targets `verify-artifacts` e `lint-workflows`. |
| `.updateGit.yaml` | 1, 3 | Chaves `git.timeout`, `log.level` e `log.color` comentadas, e exemplos de `export`/`unset` das novas variáveis. |
| `CONTRIBUTING.md` | todas | Cada mudança deve vir com testes, e `make check` precisa passar antes do commit. |
| `CONTRIBUTING.md` | 4 | Como atualizar os *golden files* (`go test ./internal/output -update`) e revisar o diff. |
| `CONTRIBUTING.md` | 5 | Novo fluxo de release por tag (seção 11.5), regra "tag = `CLIVersion`, sem `v`", pré-releases `-rc.N`, como apagar uma tag errada e o fim do upload manual de binários. |
| `Makefile` (`make help`) | 5, 6 | Novos targets com descrição `## ...` para aparecerem no `make help`. |
| `README.md` | 6 | **"Development Commands"**: `make check`, `make check-all`, `make lint`, `make vuln`, `make security`, `make install-hooks`; badge do workflow de CI. |
| `CONTRIBUTING.md` | 6 | Setup com `make install-deps && make install-hooks`; o que roda no pre-commit e no pre-push; como corrigir cada falha comum; o CI de PR é obrigatório para o merge. |
| `README.md` | 7 | Seção **"Log file"**: `-O`, `--log-file-dir`, `--log-file-level`, formato do nome, permissão `0600`, limpeza do `/tmp` pelo sistema e exemplo de uso com cron. |
| `README.md` | 8 | Seção **"Progress bar"**: quando aparece, `--progress` e por que fica desligada no modo sequencial. |
| `README.md` | 9 | Seção **"Recursive search"**: busca em até 3 níveis, padrão 2 (exemplo de árvore da seção 8.8), não entra em repositórios, descoberta antes da atualização, `-R 1` para o comportamento anterior, `--git-exclude-dirs`, nomes por caminho relativo, `skip_repos` com caminho, worktrees e links simbólicos. |
| `README.md` | 10 | Seção **"Backup"**: `--backup-max-concurrent` e recomendações para `copy` x `stash`. |
| `.updateGit.yaml` | 7-10 | Novas chaves `log.file.*`, `log.progress`, `git.max_depth`, `git.exclude_dirs`, `backup.max_concurrent`, e `export`/`unset` das variáveis correspondentes. |
| `CHANGELOG.md` | todas | Entradas na seção `0.2.0` (rascunho em 17.2) e TOC atualizado. |
| `CLAUDE.md` | todas | Ver 17.3. |
| `internal/config/config.go` | 5 | `CLIVersion = "0.2.0"`, `CLICommit` e `CLIBuildDate`. |
| Este plano | todas | Marcar os critérios de aceite concluídos. Ao terminar, mover para `plans/done/` ou registrar o status no topo. |

### 17.2 Rascunho do `CHANGELOG.md`

```markdown
# 0.2.0

## Added

- Parallel update of repositories: ``--git-max-concurrent``/``-J`` (1-100) now really runs N ``git pull`` at the same time.
- ``--git-timeout``/``-T`` (``git.timeout``, ``CLI_GIT_TIMEOUT``): timeout in seconds for each ``git pull`` (default 300).
- ``--log-level``/``-L`` (``log.level``, ``CLI_LOG_LEVEL``): minimum log level (debug, info, warning, error).
- ``--color`` (``log.color``, ``CLI_LOG_COLOR``): colored output (auto, always, never); respects ``NO_COLOR``.
- Output per repository showing the current branch (upstream, ahead/behind), the other local branches and the pull result.
- Final summary with updated, failed, timed out and skipped repositories.
- GitHub Actions workflow that builds the binaries (linux/darwin, amd64/arm64), SBOM and checksums and publishes a GitHub release when a semantic version tag is pushed.
- ``updateGit -V`` shows the commit and build date.
- Unit tests for the parallel runner, branch info, logger and output.
- ``--log-file-enabled``/``-O``, ``--log-file-dir`` (default ``/tmp``) and ``--log-file-level``: save the log to ``updateGit_YYYY-MM-DD_HH-MM-SS.log``.
- Live progress bar (``--progress``: auto, always, never) with counters, running repositories and ETA.
- Search for repositories up to 3 directory levels (``--git-max-depth``/``-R``: 1, 2 or 3, default 2) without walking inside repositories, and ``--git-exclude-dirs``; git worktrees are detected. Discovery finishes before any update starts, so the total is known for progress and logs.
- ``--backup-max-concurrent``: limit of simultaneous backups, independent of ``--git-max-concurrent``.
- Development checks in the Makefile: ``lint`` (golangci-lint), ``mod-check``, ``test-short``, ``coverage-check``, ``vuln`` (govulncheck), ``security`` (trivy), ``lint-workflows`` (actionlint), ``check-all`` and ``install-hooks`` (pre-commit).
- GitHub Actions CI for pull requests (Linux and macOS) running ``make check-all``.

## Changed

- **Behavior change:** ``updateGit pull`` runs in parallel by default (``git.parallel_enabled: true``, 10 workers). Use ``--git-parallel-enabled=false`` or ``-J 1`` for the previous sequential behavior.
- In parallel mode git runs non-interactively (no password prompts).
- Repositories whose current branch has no upstream (or detached HEAD) are skipped instead of failing.
- Log messages are colored by level and the stack trace is shown only in debug level.
- Release process: binaries are no longer uploaded manually (see CONTRIBUTING.md).
- **Behavior change:** repositories in the second directory level are found by default (``--git-max-depth=2``). Use ``-R 1`` for the previous behavior.
- Repository names are shown as the path relative to the base directory (same as before for first level repositories).
- A permission error while scanning a subdirectory is now a warning instead of a fatal error.
- ``make check`` now also runs ``mod-check`` and ``lint``.

## Fixed

- ``--git-parallel-enabled`` and ``--git-max-concurrent`` were ignored.
- Data race in the logger when called from multiple goroutines.
- Build ``LDFLAGS`` referencing nonexistent variables.
```

> O formato com subseções `Added/Changed/Fixed` é uma sugestão (inspirado em [Keep a Changelog](https://keepachangelog.com)). Se preferir manter a lista simples usada na `0.1.1`, as entradas são as mesmas.

### 17.3 O que entra no `CLAUDE.md`

- **Key files:** `internal/output/output.go` (renderização), `internal/output/progress.go` e `internal/output/terminal.go` (barra de progresso e terminal compartilhado), `internal/git/runner.go` (pool de workers, se separado), `.github/workflows/release.yml` e `plans/`.
- **Convenções novas:**
  - O logger é configurado uma vez via `common.InitLogger`. Nunca reconfigurar o `zerolog` dentro do `Logger`.
  - Workers nunca escrevem no terminal nem chamam `Logger("fatal")`: eles devolvem um `RepoResult`, e só a goroutine de saída imprime.
  - Use `pullFunc` e writers injetáveis nos testes; testes de concorrência precisam passar com `-race`.
  - *Golden files* em `internal/output/testdata`, atualizados com `-update`.
- **CLI behavior:** novas flags `-T`, `-L` e `--color`; `-D` equivale a `--log-level=debug`; códigos de saída 0, 1 e 130.
- **Configuration conventions:** novas chaves `git.timeout` e `log.level`/`log.color` e as variáveis `CLI_*` correspondentes (incluídas no `bindEnvs`).
- **Release:**
  - A tag é semver sem `v` e deve ser igual a `CLIVersion`.
  - O CI publica a release com título igual à tag e a mensagem padrão.
  - Os nomes dos assets e o formato do `checksums.txt` (`bin/` prefix) são contrato com o `updateGit update`.
- **Makefile targets:** `verify-artifacts`, `lint-workflows`, `lint`, `lint-fix`, `mod-check`, `test-short`, `coverage-check`, `vuln`, `security`, `check-all` e `install-hooks`. Regra: **rodar `make check` antes de considerar qualquer mudança pronta**, e `make check-all` antes de abrir PR.
- **Dev tooling:** `.golangci.yml` (o `common.Logger` é tratado como função printf), `.pre-commit-config.yaml`, `.github/workflows/ci.yml`; ferramentas fixadas no `mise.toml`.
- **Log em arquivo:** `/tmp/updateGit_YYYY-MM-DD_HH-MM-SS.log` com `0600`, sem ANSI; todo writer de saída passa pelo `TerminalWriter` (tela) ou pelo writer do arquivo, nunca `fmt.Print` direto.
- **Barra de progresso:** só a goroutine de saída desenha; workers enviam eventos por canal.
- **Busca de repositórios:** até 3 níveis (`-R` 1-3, default 2); a busca não entra em repositórios nem segue links simbólicos; a descoberta termina antes de qualquer atualização (`DiscoveryResult`), e o total é conhecido desde o início; `Repository.Name` é o caminho relativo ao `base_dir`.
- **Backups:** semáforo `backup.max_concurrent` adquirido só em volta do `CreateBackup`, sempre com `select` no `ctx.Done()`.

## 18. Riscos e mitigação

| Risco | Mitigação |
|---|---|
| Muitas conexões SSH simultâneas para o mesmo host (GitHub/GitLab podem recusar ou limitar; o `MaxStartups` do sshd costuma ser `10:30:100`). | Default de 10 workers e limite de 100. Documentar o uso de `ControlMaster`/`ControlPersist` no `~/.ssh/config` para reaproveitar conexões. Mensagem de erro clara quando o servidor recusar (`Connection closed by remote host`). |
| Prompts de senha em paralelo. | Modo não interativo quando `workers > 1` e sugestão de usar o modo sequencial (seção 6.4). |
| Repositório com hook ou rede lenta travando. | Timeout por repositório com `CommandContext`. |
| Saída intercalada. | Saída do git capturada e uma única goroutine imprimindo (seção 6.5). |
| *Data race* no logger. | Fase 1 (`InitLogger` com configuração única) e testes com `-race`. |
| Disco/IO com `backup.strategy=copy` em 50 repositórios ao mesmo tempo. | Limite separado `backup.max_concurrent` (Fase 10, default 4). |
| Quebra de scripts que dependem do texto atual da saída. | O código de saída continua `0` (sucesso) e `1` (falha). Registrar a mudança de formato no CHANGELOG. |
| Tag criada com versão errada ou sem bump do `CLIVersion`. | O workflow falha na validação, antes de compilar. Basta apagar a tag (`git push --delete origin <tag>`), corrigir e criar de novo. |
| Mudança no nome dos assets ou no formato do `checksums.txt` quebra o `updateGit update` das versões instaladas. | Os nomes são tratados como contrato (seção 11.2) e verificados no passo "Verify artifacts". |
| Action de terceiros comprometida. | Actions fixadas por SHA, `permissions: contents: write` só no job de release e criação da release com o `gh` nativo. |
| Alguém sem permissão cria uma tag e publica uma release. | Ruleset de proteção de tags no GitHub (seção 11.5). |
| Log em `/tmp` legível por outros usuários da máquina. | Arquivo criado com `0600`; documentar que URLs com credenciais embutidas (`https://user:token@...`) aparecem no log e devem ser evitadas. O log mascara `user:token@` como `***@`. |
| `/tmp` limpo no reboot apaga logs antigos. | Documentar e permitir `--log-file-dir` persistente (ex.: `~/.local/state/updateGit`). |
| Barra de progresso corrompendo a saída (linhas misturadas). | `TerminalWriter` único com mutex e testes de intercalação com `-race` (Fase 8). |
| Busca lenta em árvores grandes (ex.: `$HOME` como `base_dir`). | Máximo de 3 níveis (default 2), a busca não entra em repositórios, `exclude_dirs` e a duração da descoberta aparece no log. |
| Default `max_depth=2` encontra repositórios que antes eram ignorados. | Registrar como mudança de comportamento no CHANGELOG; `-R 1` volta ao comportamento anterior; `skip_repos` para excluir. |
| Mudança do nome do repositório para caminho relativo quebra `skip_repos` existentes. | Com profundidade 1 o nome não muda, e `skip_repos` aceita o nome da pasta e o caminho relativo. |
| Novos linters gerando muitos avisos de uma vez. | Corrigir o código existente dentro da Fase 6, antes das outras fases; exclusões só pontuais e comentadas. |
| `make check-all` lento ou dependente de rede no pre-push. | `make check` (rápido) no dia a dia e `check-all` no pre-push e no CI; `git push --no-verify` documentado para emergências. |

## 19. Fora do escopo (ideias futuras)

- Saída em JSON (`--output=json`) para integração com outras ferramentas.
- `git fetch --all --prune` opcional antes do pull.
- Assinatura dos binários e do `checksums.txt` (cosign/Sigstore) e atestação de proveniência (`actions/attest-build-provenance`).
- Publicação em gerenciadores de pacote (Homebrew tap, registry do mise/aqua).
- Retenção/rotação automática dos arquivos de log.
- Testes de *fuzzing* (`go test -fuzz`) para o parsing de `git for-each-ref` e do `checksums.txt`.

## 20. Critérios de aceite

- [x] `updateGit pull -J 20` executa no máximo 20 `git pull` simultâneos (verificado em teste).
- [x] `--git-parallel-enabled=false` ou `-J 1` mantêm o comportamento sequencial, com prompt interativo de senha.
- [x] A falha de um repositório não interrompe os outros, o resumo lista as falhas e o código de saída é `1`.
- [x] O timeout por repositório funciona e aparece no resumo.
- [x] `--log-level` filtra as mensagens; `-D` continua funcionando.
- [x] Cores por nível em terminal; nenhuma sequência ANSI com `--color=never`, `NO_COLOR` ou saída redirecionada.
- [x] Cada bloco mostra a branch atual com upstream e estado, e as demais branches locais.
- [x] `make check` passa (gofmt, vet e testes com `-race`) e `make build` gera os binários.
- [ ] Push de uma tag semântica (ex.: `0.2.0`) dispara o workflow **Release**, que publica uma release com **título igual à tag**, a mensagem `See changes in [CHANGELOG.md](https://github.com/aeciopires/updateGit/blob/main/CHANGELOG.md) file` e os assets `updateGit-{linux,darwin}-{amd64,arm64}`, `checksums.txt` e `updateGit.sbom.json`. *(Pendente: depende do push para o GitHub; workflow validado localmente com actionlint e `make check-all`.)*
- [ ] Tag fora do padrão semântico ou diferente de `config.CLIVersion` não gera release. *(Pendente: depende do push para o GitHub; workflow validado localmente com actionlint e `make check-all`.)*
- [ ] Tag de pré-release (`-rc.N`) é publicada como *pre-release* e não é oferecida pelo `updateGit update`. *(Pendente: depende do push para o GitHub; workflow validado localmente com actionlint e `make check-all`.)*
- [ ] `updateGit update` a partir da versão anterior baixa e valida a nova versão publicada pelo CI. *(Pendente: depende do push para o GitHub; workflow validado localmente com actionlint e `make check-all`.)*
- [x] Todos os testes da seção 10 implementados e passando em `make check` (com `-race`), sem queda de cobertura nos pacotes existentes e com `internal/output` ≥ 85%.
- [x] Testes existentes ajustados (seção 10.7) e teste manual de carga registrado no PR.
- [x] README, CONTRIBUTING, `.updateGit.yaml` e `make help` atualizados conforme a seção 17.1.
- [x] `CHANGELOG.md` com a seção `0.2.0` (seção 17.2), incluindo a mudança de comportamento, e o TOC atualizado.
- [x] `CLAUDE.md` atualizado conforme a seção 17.3.
- [x] **Fase 6:** `make check` (fmt, mod, vet, lint, testes com `-race`) e `make check-all` (+ cobertura mínima, govulncheck, trivy, actionlint, build e artefatos) passam; os hooks de pre-commit/pre-push barram os erros propositais da seção 10.8; o CI de PR roda em Linux e macOS e é obrigatório para o merge. *(Workflow criado e validado com actionlint; executar no GitHub e tornar obrigatório depende do push e da branch protection.)*
- [x] **Fase 7:** `-O` grava o log em `/tmp/updateGit_YYYY-MM-DD_HH-MM-SS.log` (ou outro diretório com `--log-file-dir`), com `0600`, sem ANSI, com blocos e resumo, e o caminho é informado ao final.
- [x] **Fase 8:** a barra de progresso aparece só em terminal com `workers > 1`, não corrompe a saída, não vai para o arquivo de log e some antes do resumo.
- [x] **Fase 9:** a descoberta termina antes do primeiro backup/pull, e o total aparece nos logs, no `[n/N]` e na barra; por padrão a busca encontra repositórios nos níveis 1 e 2, com `-R 3` também no nível 3 e nunca no nível 4; não entra em pastas que são repositórios, respeita `exclude_dirs`, não segue links simbólicos e reconhece worktrees; com `-R 1`, o resultado é idêntico ao da versão anterior.
- [x] **Fase 10:** com `-J 20 --backup-max-concurrent 3`, no máximo 3 backups e 20 pulls simultâneos (verificado em teste).
- [x] Versão `0.2.0` em `config.CLIVersion`.

## 21. Ajustes após a implementação

Registro das mudanças feitas depois da implementação original (ainda na versão `0.2.0`, não publicada). As seções anteriores foram mantidas como estavam no plano aprovado.

### 21.1 Profundidade da busca até 6 níveis

- `--git-max-depth`/`-R` passou a aceitar de `1` a `6` (antes `1` a `3`). O padrão continua `2`.
- As regras não mudaram: a descoberta termina antes de qualquer atualização, a busca nunca entra em pastas que são repositórios, ignora diretórios ocultos e `exclude_dirs` e não segue links simbólicos.
- Alterados: `git.MaxSearchDepth = 6`, a validação `max=6` de `Git.MaxDepth` em `config.Config` e a ajuda da flag.
- Testes: `TestFindRepositoriesDepth` usa uma árvore com repositórios nos níveis 1 a 7 (`max_depth` de 1 a 6 encontra os níveis correspondentes, o nível 7 nunca é encontrado e o valor 7 é reduzido a 6); `TestFindRepositoriesDoesNotWalkInsideRepo` confere os diretórios lidos com a profundidade máxima; `TestMaxDepthValidation` aceita de 1 a 6 e rejeita 0 e 7; `TestConfigErrors` rejeita `-R 7`.
- Documentação: README (Features, Usage e "Recursive search", com um aviso para não usar `-R 6` em diretórios grandes como `$HOME`), `.updateGit.yaml`, CLAUDE.md e CHANGELOG.

### 21.2 Pasta `docs/`

Documentação para leigos, sem explicar código, com diagramas Mermaid:

| Arquivo | Conteúdo |
|---|---|
| `docs/README.md` | Índice, visão geral em um diagrama e glossário (repositório, remoto, branch, upstream, ahead/behind, worker, timeout...). |
| `docs/how-it-works.md` | Uma execução passo a passo: configurações e precedência, descoberta, filtro, workers em paralelo, o que acontece com cada repositório (fluxo e estados), backups, progresso, log em arquivo, relatório final, códigos de saída e regras de segurança. |
| `docs/architecture.md` | Blocos da arquitetura e responsabilidades, mapa das pastas, conversa entre os blocos durante um `pull` (diagrama de sequência), modelo de paralelismo (fila, caixa de resultados, vagas de backup, porteiro da tela), configurações, tela x arquivo de log, self-update, checagens de qualidade e releases, e princípios de projeto. |

- O README ganhou a seção "Documentation" com os links, e o CLAUDE.md pede para manter `docs/` atualizada quando o fluxo ou os componentes mudarem.
- Os diagramas foram revisados manualmente nesta etapa e validados depois com Docker (seção 21.4).

### 21.3 Pular repositórios com arquivos não commitados (padrão)

- Para evitar perda de trabalho, um repositório cuja branch atual tem arquivos não commitados (modificados, adicionados com `git add`, novos não ignorados pelo `.gitignore` ou em conflito) é **pulado por padrão**, com status `SKIPPED` e motivo `uncommitted changes`.
- A verificação (`git status --porcelain`, função `GetWorkingTreeStatus` em `internal/git/worktree.go`) roda depois da checagem de branch/upstream e **antes do backup e do pull**. O bloco mostra o que foi encontrado (ex.: `2 modified, 1 untracked`) e uma dica. Pular não é falha: o código de saída continua `0`.
- Nova opção para desativar: `--git-skip-dirty=false` (`git.skip_dirty`, `CLI_GIT_SKIP_DIRTY`), que volta ao comportamento anterior e mostra um aviso. Recomendado junto com `-B -Y stash`.
- Testes: `TestGetWorkingTreeStatus` (limpo, ignorado, modificado, staged, untracked, staged+modificado), `TestGetWorkingTreeStatusConflict`, `TestGetWorkingTreeStatusNotARepo`, `TestSkipDirtyRepository` (nenhum pull nem backup nos repositórios sujos; com a opção desativada todos são atualizados), `TestSkipDirtyStatusError`, `TestSkipDirtyDefaultAndFlag`, `TestPullSkipsDirtyRepositories` (o trabalho local é preservado) e golden files `block-skipped-dirty-*` e do resumo.
- Documentação: README (seção "Files not committed", Features, Usage, configuração e variáveis), `.updateGit.yaml`, `docs/how-it-works.md` (fluxo por repositório, estados e regras de segurança), `docs/architecture.md`, CLAUDE.md e CHANGELOG.

### 21.4 Validação dos diagramas Mermaid com Docker

- Os 16 diagramas da pasta `docs/` foram renderizados com a imagem `minlag/mermaid-cli:12.0.0` (versão fixada; a `12.0.1` tinha sido publicada poucos dias antes). Todos são válidos.
- Conferência visual: o diagrama geral da arquitetura tinha o bloco "Communication" no topo por causa de uma seta de volta para o usuário; ele passou a terminar em um nó "Your screen", deixando o fluxo de cima para baixo.
- Novo target `make lint-diagrams` (variável `MERMAID_IMAGE`): extrai os blocos `mermaid` de todos os arquivos Markdown e valida cada um com a imagem Docker; sem Docker, avisa e pula. Faz parte do `make check-all` (pre-push e CI). Testado com um diagrama inválido proposital.

### 21.5 Boas práticas e segurança

- **Self-update (`internal/update`)**:
  - Downloads só por HTTPS e só de hosts do GitHub, inclusive nos redirecionamentos.
  - Timeouts (API 30s, download 5min), limite de 256 MiB por download e cabeçalhos `User-Agent`/API do GitHub.
  - Validação do formato SHA256, com a verificação do checksum feita antes de tocar no binário atual.
  - Só instala versões **mais novas**, com comparação semver que considera pré-releases. Antes, qualquer tag diferente da versão instalada era instalada, mesmo mais antiga.
  - Funções que retornam erros em vez de encerrar o processo (`LatestRelease`, `ApplyUpdate`, `ReplaceBinary`); o comando `update` decide a saída (código 1).
- **Timeouts e contexto**: os comandos git de leitura de cada repositório (branches e status) usam o `git.timeout`, e os comandos do backup têm um timeout de 2 minutos.
- **Linter mais rígido**: `noctx`, `nilerr`, `nolintlint` (todo `//nolint` precisa nomear o linter e justificar), `errname`, `durationcheck` e `usestdlibvars`. Diretivas `//nolint` desnecessárias foram removidas.
- **GitHub Actions**: `persist-credentials: false` no checkout.
- **Documentação**: `SECURITY.md` (política e reporte privado de vulnerabilidades) e uma seção "Security" no CONTRIBUTING.

### 21.6 Cobertura de testes

- Refatorações para permitir testes sem encerrar o processo: `exitFunc` em `cmd`, funções substituíveis no comando `update` (`latestRelease`, `applyUpdate`, `executablePath`) e `ApplyUpdate` recebendo o caminho do executável.
- Novos testes:
  - `Execute` dentro do processo (`-v`, `-V`, ajuda, `pull` com log em arquivo, erro de flag e erro do log em arquivo) e o comando `update` (já atualizado, versão antiga, confirmado, cancelado e erros).
  - `ApplyUpdate` completo com servidor HTTP local (sucesso, binário adulterado, checksum ausente, assets ausentes, erros de download), `LatestRelease`, `CompareVersions`/`IsNewerVersion`, `ReplaceBinary` e validação de URLs e redirecionamentos.
  - Caminhos de erro do backup (arquivo ilegível, destino sem permissão, stash fora de repositório) e mensagens de validação.
- Resultado: cobertura total de **88,8% para 95,4%**. Por pacote: `cmd` 71% → 91%, `update` 17% → 94%, `backup` 81% → 91%, `common` 90% → 96%, `git` 96% → 98% e `output` 97% → 98%.
- O mínimo do `make coverage-check` passou de 70% para **90%**.
- Ficam sem teste apenas o `main()` e os ramos que dependem de terminal real ou de sistema operacional não suportado.
