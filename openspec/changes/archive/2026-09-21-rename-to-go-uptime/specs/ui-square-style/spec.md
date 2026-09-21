## MODIFIED Requirements

### Requirement: Badges SVG sem cantos arredondados
Os badges SVG servidos pela API (saúde, uptime e tempo de resposta) MUST ser gerados sem cantos arredondados. O endpoint `badge.shields`, que devolve dados para o shields.io renderizar, MUST NOT ter a forma da resposta alterada por esta regra; o rótulo que ele devolve é o nome do projeto (`go-uptime`), definido em `project-identity`.

#### Scenario: Badge de saúde
- **WHEN** um cliente requisita `GET /api/v1/endpoints/core_api/health/badge.svg`
- **THEN** nenhum elemento `rect` do SVG tem atributo `rx` maior que zero
