## MODIFIED Requirements

### Requirement: Limites e ciclo de vida das conexões de eventos
O sistema MUST limitar as conexões de eventos abertas a 500 no total e a 10 por IP de cliente, calculado com `status-pages.trusted-proxies`, somando as duas rotas. Acima do limite, MUST responder 429 com `Retry-After: 30`, `Cache-Control: no-store` e `{"error":"too many requests"}`.

**Liberação da vaga:** uma conexão encerrada pelo cliente, por erro de escrita, pelo tempo máximo, por pânico no envio ou por falha antes de começar o envio MUST liberar a vaga. Um pânico no envio MUST NOT derrubar o processo.

**Parada:** numa recarga da configuração ou no desligamento, o sistema MUST encerrar as conexões de eventos antes de parar o servidor. A parada MUST NOT esperar mais de 10 segundos pelas conexões, e novas conexões MUST ser recusadas com 503 enquanto ela dura. Depois da recarga, o servidor novo MUST aceitar conexões de eventos assim que começar a escutar.

**Chaves esquecidas:** remover ou renomear um endpoint, ou tirá-lo da configuração numa recarga bem-sucedida, MUST descartar a sequência da chave.

#### Scenario: Limite por IP
- **WHEN** um IP já tem 10 conexões de eventos abertas e abre a 11ª
- **THEN** a resposta é 429 com `Retry-After: 30`

#### Scenario: Cliente que foi embora
- **WHEN** um IP fecha as suas 10 conexões sem avisar
- **THEN** em até 30 segundos o IP consegue abrir novas conexões

#### Scenario: Recarga com conexões abertas
- **WHEN** há 20 conexões de eventos abertas e o arquivo de configuração é recarregado
- **THEN** as conexões são encerradas e a recarga não espera 5 minutos

#### Scenario: Atrás do nginx sem trusted-proxies
- **WHEN** o Go Uptime está atrás de um proxy que não está em `status-pages.trusted-proxies` e 11 visitantes abrem os detalhes
- **THEN** o 11º recebe 429 no canal e continua com a atualização periódica
- **AND** a documentação indica configurar `trusted-proxies`

### Requirement: Detalhes do endpoint em tempo real
A página de detalhes do endpoint no dashboard e a página pública de detalhes MUST abrir o canal de eventos do endpoint e, a cada aviso, atualizar sem indicador de carregamento as barras, o painel de números, o gráfico **Response Time Trend** e a tabela de checks. No dashboard, barras, painel e gráfico MUST mostrar sempre os resultados mais recentes, mesmo com a tabela em outra página de resultados.

- **Ordem das respostas:** uma resposta mais antiga que chegue depois de uma mais nova MUST ser descartada.
- **Aba oculta:** a página MUST fechar o canal quando a aba ficar oculta e, ao voltar, MUST reabri-lo e atualizar uma vez.
- **Troca de endpoint:** a troca de endpoint na mesma tela MUST reabrir o canal para o endpoint novo.
- **Canal fechado por erro:** se o canal ficar fechado por erro (502 do proxy numa recarga, 503, 429, 404 ou 401), a página MUST continuar com a atualização periódica e MUST reabrir o canal com espera crescente de 30 segundos até 5 minutos, ou logo depois de uma atualização periódica bem-sucedida, informando o último `id` recebido. Numa queda depois de uma resposta 200, a página MUST deixar a reconexão automática do navegador agir, sem abrir uma segunda conexão.
- **Gráfico:** no período Recent, o gráfico MUST recarregar logo depois de cada atualização dos dados da página; nos períodos 3h, 6h, 24h e 1w, MUST recarregar no máximo uma vez a cada 60 segundos. Nas atualizações, o gráfico MUST NOT ser apagado nem mostrar indicador de carregamento, e um aviso que chegue durante uma carga MUST gerar uma nova busca quando ela terminar.

#### Scenario: Pending no gráfico na hora
- **WHEN** o administrador está nos detalhes de `jobs_backup` e um script envia `status=pending`
- **THEN** em até 2 segundos, a barra, a tabela e, com o gráfico em Recent, a coluna amarela do gráfico aparecem, sem recarregar a página

#### Scenario: Página pública em tempo real
- **WHEN** um visitante está em `/status/jobs/endpoints/jobs_backup` e o endpoint recebe um push
- **THEN** a página mostra o resultado novo em até 2 segundos, mesmo com a resposta de detalhes ainda em cache

#### Scenario: Recarga do Go Uptime atrás do nginx
- **WHEN** o visitante está nos detalhes, a configuração do Go Uptime é recarregada e o canal recebe 502 do nginx
- **THEN** a página continua mostrando os dados e volta a receber avisos em no máximo 5 minutos, sem ação do visitante

#### Scenario: Pending com a tabela na página 2
- **WHEN** o administrador está nos detalhes de `jobs_backup` com a tabela na página 2 e chega `status=pending`
- **THEN** as barras, o painel e, com o gráfico em Recent, a coluna amarela do gráfico mostram o Pending em até 2 segundos

#### Scenario: Canal indisponível
- **WHEN** a abertura do canal responde 429
- **THEN** a página continua mostrando os dados e se atualiza no intervalo periódico
