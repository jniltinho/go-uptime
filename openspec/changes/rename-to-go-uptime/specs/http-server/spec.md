## MODIFIED Requirements

### Requirement: Esquema e sessão sem confiar em cabeçalhos
A proteção contra CSRF e o atributo `Secure` do cookie de sessão MUST continuar usando o TLS da própria conexão, o `Host` da requisição e o **primeiro valor** de `X-Forwarded-Proto`, exatamente como hoje, e MUST NOT usar a resolução de esquema do framework, que também confia em `X-Forwarded-Protocol`, `X-Forwarded-Ssl` e `X-Url-Scheme`, nem `X-Forwarded-Host`. O cookie `go_uptime_session` MUST manter `Path=/`, `HttpOnly` e `SameSite=Strict`.

#### Scenario: Proxy que termina o TLS
- **WHEN** uma conexão sem TLS envia `X-Forwarded-Proto: https` no login
- **THEN** o cookie de sessão é emitido com `Secure`, como hoje

#### Scenario: Cabeçalhos que o framework aceitaria
- **WHEN** uma conexão sem TLS envia só `X-Forwarded-Ssl: on` no login
- **THEN** o cookie de sessão é emitido sem `Secure`

#### Scenario: Host forjado
- **WHEN** chega um `POST` da administração com `Origin: https://evil.example` e `X-Forwarded-Host: evil.example`
- **THEN** a resposta é 403
