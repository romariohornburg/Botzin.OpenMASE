package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/iagoMAO/Botzin.OpenMASE/authentication"
	"github.com/iagoMAO/Botzin.OpenMASE/buddy"
	"github.com/iagoMAO/Botzin.OpenMASE/core"
	"github.com/iagoMAO/Botzin.OpenMASE/database"
	"github.com/iagoMAO/Botzin.OpenMASE/items"
	"github.com/iagoMAO/Botzin.OpenMASE/progression"
	"github.com/iagoMAO/Botzin.OpenMASE/protocol"
	"github.com/iagoMAO/Botzin.OpenMASE/utils"
)

type command struct {
	usage   string
	help    string
	minArgs int
	run     func(out io.Writer, args []string) error
}

var commands map[string]command

func init() {
	commands = map[string]command{
		"help": {"help()", "lista os comandos", 0, cmdHelp},
		"broadcast": {"broadcast(mensagem[, cor])", "anúncio para todos os jogadores online", 1, func(out io.Writer, args []string) error {
			color := ""
			if len(args) > 1 {
				color = args[1]
			}
			fmt.Fprintf(out, "Anúncio enviado para %d jogador(es).\n", buddy.BroadcastMessage(args[0], color))
			return nil
		}},
		"msg": {"msg(nick, mensagem)", "anúncio para um jogador", 2, func(out io.Writer, args []string) error {
			account, err := authentication.FindAccount(args[0])
			if err != nil {
				return err
			}
			if !buddy.SendMessage(account.Id, args[1], "") {
				return errors.New("jogador offline")
			}
			fmt.Fprintln(out, "Mensagem enviada.")
			return nil
		}},
		"register": {"register(nick, senha)", "cria uma conta", 2, func(out io.Writer, args []string) error {
			id, err := authentication.Register(args[0], args[1], cfg.STARTING_CREDITS, cfg.STARTING_GOLD)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "Conta %s criada (id %d).\n", args[0], id)
			return nil
		}},
		"setpass": {"setpass(nick, senha)", "troca a senha de uma conta", 2, func(out io.Writer, args []string) error {
			account, err := authentication.FindAccount(args[0])
			if err != nil {
				return err
			}
			if err := authentication.SetPassword(account.Id, args[1]); err != nil {
				return err
			}
			fmt.Fprintf(out, "Senha de %s alterada.\n", account.Nick)
			return nil
		}},
		"online": {"online()", "lista os jogadores conectados", 0, cmdOnline},
		"info":   {"info(nick)", "mostra os dados de uma conta", 1, cmdInfo},
		"kick": {"kick(nick)", "desconecta um jogador", 1, func(out io.Writer, args []string) error {
			account, err := authentication.FindAccount(args[0])
			if err != nil {
				return err
			}
			session := core.GetSessionByUserId(account.Id)
			if session == nil {
				return errors.New("jogador offline")
			}
			session.Close()
			fmt.Fprintf(out, "%s desconectado.\n", account.Nick)
			return nil
		}},
		"ban":   {"ban(nick)", "bane (e desconecta) uma conta", 1, func(out io.Writer, args []string) error { return setBan(out, args[0], true) }},
		"unban": {"unban(nick)", "remove o banimento", 1, func(out io.Writer, args []string) error { return setBan(out, args[0], false) }},
		"give":  {"give(nick, itemId)", "dá um item a uma conta", 2, cmdGive},
		"credits": {"credits(nick, quantidade)", "adiciona (ou remove, se negativo) créditos", 2, func(out io.Writer, args []string) error {
			return addCurrency(out, args, "credits")
		}},
		"gold": {"gold(nick, quantidade)", "adiciona (ou remove, se negativo) gold", 2, func(out io.Writer, args []string) error {
			return addCurrency(out, args, "gold")
		}},
		"xp": {"xp(nick, quantidade)", "adiciona (ou remove) XP, recalculando nível e patente", 2, func(out io.Writer, args []string) error {
			account, amount, err := accountAndInt(args)
			if err != nil {
				return err
			}
			xp, level, err := progression.AddXP(account.Id, amount)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "%s agora tem %d XP (nível %d, patente %d).\n", account.Nick, xp, level, progression.PatentForLevel(level))
			return nil
		}},
		"ranking": {"ranking([quantidade])", "mostra o topo do ranking", 0, cmdRanking},
		"lobbies": {"lobbies()", "lista os servidores de partida ativos", 0, cmdLobbies},
		"items":   {"items()", "lista o catálogo de itens", 0, cmdItems},
		"reloaditems": {"reloaditems()", "recarrega o items.sql (mantém os inventários)", 0, func(out io.Writer, args []string) error {
			loaded, err := database.LoadItems(cfg.ITEMS_SQL_PATH, true)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "%d itens carregados.\n", loaded)
			return nil
		}},
	}
}

var callPattern = regexp.MustCompile(`^([a-zA-Z0-9_]+)\s*\((.*)\)$`)

// parseCommand accepts both "cmd(arg1, "arg 2")" and "cmd arg1 arg2".
func parseCommand(line string) (string, []string, error) {
	line = strings.TrimSpace(line)

	if matches := callPattern.FindStringSubmatch(line); matches != nil {
		argsStr := strings.TrimSpace(matches[2])
		if argsStr == "" {
			return strings.ToLower(matches[1]), nil, nil
		}

		reader := csv.NewReader(strings.NewReader(argsStr))
		reader.TrimLeadingSpace = true
		reader.LazyQuotes = true

		args, err := reader.Read()
		if err != nil {
			return "", nil, fmt.Errorf("argumentos inválidos: %w", err)
		}

		for i := range args {
			args[i] = strings.TrimSpace(args[i])
		}

		return strings.ToLower(matches[1]), args, nil
	}

	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", nil, nil
	}

	return strings.ToLower(fields[0]), fields[1:], nil
}

func runCommand(out io.Writer, line string) {
	defer utils.Recover("console command")

	name, args, err := parseCommand(line)
	if err != nil {
		fmt.Fprintln(out, err)
		return
	}

	if name == "" {
		return
	}

	cmd, ok := commands[name]
	if !ok {
		fmt.Fprintf(out, "Comando desconhecido: %s (digite help)\n", name)
		return
	}

	if len(args) < cmd.minArgs {
		fmt.Fprintf(out, "Uso: %s\n", cmd.usage)
		return
	}

	if err := cmd.run(out, args); err != nil {
		fmt.Fprintf(out, "Erro: %s\n", err)
	}
}

func commandInput(rl *readline.Instance) {
	for {
		line, err := rl.Readline()
		if err != nil {
			break
		}

		runCommand(rl.Stdout(), line)
	}
}

func cmdHelp(out io.Writer, args []string) error {
	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		fmt.Fprintf(out, "  %-28s %s\n", commands[name].usage, commands[name].help)
	}
	return nil
}

func statusName(session *core.Session) string {
	if !session.HasBuddy() && session.MASEAddr() == nil {
		return "offline"
	}

	switch session.GetStatus() {
	case protocol.BUDDY_STATUS_INGAME:
		return "em jogo"
	default:
		return "online"
	}
}

func cmdOnline(out io.Writer, args []string) error {
	sessions := core.GetAllSessions()
	fmt.Fprintf(out, "%d jogador(es) conectado(s):\n", len(sessions))

	for _, session := range sessions {
		nick := authentication.GetUserInfo(session.UserId).Nick
		addr := "-"
		if a := session.MASEAddr(); a != nil {
			addr = a.String()
		}
		fmt.Fprintf(out, "  [%d] %-16s %-8s %s buddy=%t\n", session.UserId, nick, statusName(session), addr, session.HasBuddy())
	}
	return nil
}

func cmdInfo(out io.Writer, args []string) error {
	a, err := authentication.FindAccount(args[0])
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "[%d] %s  banido=%t  online=%t\n", a.Id, a.Nick, a.Banned, core.IsLoggedIn(a.Id))
	fmt.Fprintf(out, "  XP %d | nível %d | patente %d | pontos %d | ranking %d\n", a.XP, a.Level, a.Patent, a.Points, a.Ranking)
	fmt.Fprintf(out, "  créditos %d | gold %d\n", a.Credits, a.Gold)
	fmt.Fprintf(out, "  partidas %d | vitórias %d | kills %d | mortes %d\n", a.Matches, a.Wins, a.Kills, a.Deaths)

	owned, equipped, err := items.Owned(database.DB, a.Id)
	if err != nil {
		return err
	}

	ids := make([]int, 0, len(owned))
	for id := range owned {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	for _, id := range ids {
		marker := " "
		if equipped[id] {
			marker = "*"
		}
		fmt.Fprintf(out, "  %s %3d %s\n", marker, id, owned[id].Name)
	}
	return nil
}

func setBan(out io.Writer, nick string, banned bool) error {
	account, err := authentication.FindAccount(nick)
	if err != nil {
		return err
	}

	if err := authentication.SetBanned(account.Id, banned); err != nil {
		return err
	}

	if banned {
		if session := core.GetSessionByUserId(account.Id); session != nil {
			session.Close()
		}
	}

	if err := progression.RecalculateRanking(); err != nil {
		return err
	}

	fmt.Fprintf(out, "%s banido=%t.\n", account.Nick, banned)
	return nil
}

func accountAndInt(args []string) (authentication.Account, int, error) {
	account, err := authentication.FindAccount(args[0])
	if err != nil {
		return account, 0, err
	}

	amount, err := strconv.Atoi(args[1])
	if err != nil {
		return account, 0, fmt.Errorf("quantidade inválida: %s", args[1])
	}

	return account, amount, nil
}

func addCurrency(out io.Writer, args []string, column string) error {
	account, amount, err := accountAndInt(args)
	if err != nil {
		return err
	}

	balance, err := authentication.AddCurrency(account.Id, column, amount)
	if err != nil {
		return err
	}

	if session := core.GetSessionByUserId(account.Id); session != nil {
		sendUserData(session)
	}

	fmt.Fprintf(out, "%s agora tem %d de %s.\n", account.Nick, balance, column)
	return nil
}

func cmdGive(out io.Writer, args []string) error {
	account, itemId, err := accountAndInt(args)
	if err != nil {
		return err
	}

	item, err := items.Get(itemId)
	if err != nil {
		return err
	}

	owned, _, err := items.Owned(database.DB, account.Id)
	if err != nil {
		return err
	}

	if _, ok := owned[itemId]; ok {
		return fmt.Errorf("%s já tem %s", account.Nick, item.Name)
	}

	if err := items.Grant(database.DB, account.Id, item, false); err != nil {
		return err
	}

	fmt.Fprintf(out, "%s recebeu %s.\n", account.Nick, item.Name)
	return nil
}

func cmdRanking(out io.Writer, args []string) error {
	limit := 10
	if len(args) > 0 {
		if n, err := strconv.Atoi(args[0]); err == nil && n > 0 {
			limit = n
		}
	}

	entries, err := progression.TopRanking(limit)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		fmt.Fprintf(out, "  %3dº %-16s %8d pts  nível %d\n", entry.Position, entry.Nick, entry.Points, entry.Level)
	}
	return nil
}

func cmdLobbies(out io.Writer, args []string) error {
	lobbies := GetLobbies()
	fmt.Fprintf(out, "%d lobby(s):\n", len(lobbies))

	for _, lobby := range lobbies {
		fmt.Fprintf(out, "  %-21s nível %-3d %d/%d jogadores, %d bots, visto há %s\n",
			lobby.Address, lobby.Level, lobby.PlayerCount, lobby.MaxPlayers, lobby.BotCount,
			time.Since(time.Unix(lobby.LastSeen, 0)).Round(time.Second))
	}
	return nil
}

func cmdItems(out io.Writer, args []string) error {
	all, err := items.GetAll()
	if err != nil {
		return err
	}

	for _, item := range all {
		fmt.Fprintf(out, "  %3d %-22s classe %d  payload %2d  G%d  %7d créditos  %d gold  venda=%t\n",
			item.Id, item.Name, item.Class, item.Payload, item.TheGen, item.Credits, item.Gold, item.CanSell)
	}
	return nil
}
