## MODIFIED Requirements

### Requirement: Testes ponta a ponta das status pages
O roteiro `test/e2e/status-pages.sh` MUST subir o Go Uptime compilado localmente com SQLite temporário, `security.basic` e `admin.enabled` e usar o `agent-browser` para:
- criar uma página pelas telas, pré-visualizar e publicar;
- abrir `/status/<slug>` numa sessão sem credenciais e conferir pela lista de requisições que nenhuma foi a `/api/v1/config` e nenhuma recebeu 401;
- abrir `/status/a%2Fb` e `/status/nao-existe` na mesma sessão e conferir "Página não encontrada" sem 401;
- conferir 404 da API para página desabilitada;
- simular OIDC sem sessão respondendo `/api/v1/config` com `{"oidc":true,"authenticated":false}`, conferir que a página pública não busca a configuração nem mostra a tela de login e, como controle, que `/` mostra "Login with OIDC";
- capturar a página pública nos temas claro, escuro e Bio, também com 390 px de largura, e com 360 px no tema Bio com o seletor de tema aberto;
- criar pela tela uma página que exige login e conferir, com requisições diretas e sem credencial, que a rota HTML, a API da página, os detalhes, os eventos, o gráfico e o badge da página respondem 401 com `WWW-Authenticate` e `Cache-Control: no-store`, e que com a credencial certa respondem 200;
- conferir que o detalhe da página na administração não traz o hash da credencial e que a lista marca a página como protegida;
- conferir que a página pública sem login continua respondendo 200 sem credencial;
- conferir, em `/status/messages`, que a linha de um endpoint de grupo cabe no teto de altura, que as colunas de uptime dos dois endpoints do grupo têm valor visível e começam na mesma posição, e que os rótulos dos períodos aparecem só no cabeçalho do grupo;
- conferir que o tooltip da última barra aparece sem mudar a altura da linha nem a posição da linha seguinte, sem capturar o ponteiro, dentro da largura da linha e sem rolagem horizontal, e que a região `aria-live` do detalhe existe vazia antes de qualquer barra ficar ativa;
- conferir a ausência de rolagem horizontal em 390 px e em 360 px, com o rótulo do período visível na linha do endpoint;
- conferir, na edição de uma página com mais endpoints do que cabem na coluna, que a tela não rola, que Back, Validate, Preview e Save ficam dentro da janela e que a lista de endpoints rola por dentro;
- conferir a pré-visualização no diálogo, fechada por Esc sem perder o que foi digitado, antes de continuar a usar o formulário;
- conferir o toast de informação da validação e a legenda do gráfico da página pública, pelos valores de `data-series`.

As capturas MUST ficar em `dist/prints/status-pages/`, fora do git.

#### Scenario: Execução do roteiro
- **WHEN** um desenvolvedor executa `test/e2e/status-pages.sh` com `agent-browser` e Chrome instalados
- **THEN** todas as etapas passam
- **AND** as capturas ficam em `dist/prints/status-pages/`
