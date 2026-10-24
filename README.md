# Botzin.OpenMASE
Reimplementação/emulador de servidor do finado MMO brasileiro Botzin!!! da Green Land Studios. 

<p align="left">
  <img style="max-height: 128px" src="docs/img/screenshot6.png" alt="Botzin!!! 1.17">
  <img style="max-height: 128px" src="docs/img/screenshot2.png" alt="Botzin!!! 1.17">
</p>

<p align="left">
  <img style="max-height: 256px" src="docs/img/screenshot4.png" alt="Botzin!!! 1.17">
  <img style="max-height: 256px" src="docs/img/screenshot5.png" alt="Botzin!!! 1.17">
</p>

<br>

# Funcionalidades
- [x] <b>Criptografia (MD5/XTEA)</b>
- [x] <b>Autenticação (Login/Cadastro)</b> — cadastro via página web (`REGISTER_PORT`) ou console, bloqueio de login duplicado e contas banidas
- [x] <b>Inventário (Avatar/Loja)</b> — compra, venda, presente* e preço/estoque por item; validação de equipamento (payload do bot) e de atributos
- [x] <b>MasterServer (Lista de Lobby)</b>
- [x] <b>Broadcasts (Anûncios Globais)</b>
- [x] <b>Buddylist (Lista de Amigos)</b> — login autenticado por token de sessão
- [x] <b>Progressão (Pontuação por Rounds, XP, etc...)</b> — relatório de fim de partida enviado pelo host; XP, créditos, nível, patente e ranking global
- [x] <b>Console de administração</b>

<sub>Os formatos de pacote foram confirmados com engenharia reversa dos scripts TorqueScript (DSO) embutidos no cliente 1.17. * O cliente 1.17 define o pacote de presente mas nunca o envia; o servidor o suporta para clientes/ferramentas futuras.</sub>

<br>

# Executando
```sh
cd src
cp .env.example .env
go run .
```
O catálogo de itens (`sql/items.sql`) é carregado automaticamente quando o banco está vazio. Todas as opções estão documentadas no `.env.example`.

## Console
Os comandos aceitam `comando(arg1, "arg 2")` ou `comando arg1 arg2`. Digite `help` para a lista completa:

| Comando | Descrição |
|---|---|
| `register(nick, senha)` / `setpass(nick, senha)` | cria conta / troca senha |
| `online()` / `info(nick)` | jogadores conectados / dados, estatísticas e inventário |
| `kick(nick)` / `ban(nick)` / `unban(nick)` | moderação |
| `credits(nick, qtd)` / `gold(nick, qtd)` / `xp(nick, qtd)` / `give(nick, itemId)` | ajustes de conta |
| `broadcast(mensagem[, cor])` / `msg(nick, mensagem)` | anúncios |
| `ranking([n])` / `lobbies()` / `items()` / `reloaditems()` | consultas e recarga do catálogo |

## Testes
```sh
cd src
go test -race ./...
```

<br>

# Protocolos Suportados
<b>Oficialmente</b>, o <i>OpenMASE</i> suporta as versões <b>1.03</b> (Beta) e <b>1.17</b>. (Última versão lançada)

<br>

# Licença
OpenMASE é <i>software</i> livre: você pode redistribuir e/ou modificar o código seguindo os termos da <i>GNU General Public License (v3)</i> assim como escrito pela <i>Free Software Foundation</i>.

<br>

# Disclaimer

Botzin!!! é propriedade intelectual da Green Land Studios fundada por Wagner Carvalho e Vitor Rodrigues. 
<br>
<i>OpenMASE</i> foi desenvolvido de forma totalmente independente através da engenheria reversa do(s) cliente(s) do jogo em fins <b>não monetários</b> de preservação de <i>abandonware</i>.
