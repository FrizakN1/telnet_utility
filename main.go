package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/ziutek/telnet"
)

const (
	timeout = 5 * time.Second
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatalln(err)
		return
	}

	ips := readLines("ips.txt")
	commands := readLines("commands.txt")

	commandDelay, err := strconv.Atoi(os.Getenv("COMMAND_DELAY"))
	if err != nil {
		log.Fatalln(err)
	}

	for _, ip := range ips {
		fmt.Println("Подключаемся к", ip)
		err := handleSwitch(ip, commands, commandDelay)
		if err != nil {
			log.Printf("[%s] ошибка: %v\n", ip, err)
		} else {
			fmt.Printf("[%s] успешно\n", ip)
		}
	}

	fmt.Println("Готово.")
}

func handleSwitch(ip string, commands []string, commandDelay int) error {
	file, err := os.Create("results/" + ip + ".txt")
	if err != nil {
		return err
	}
	defer file.Close()

	conn, err := telnet.Dial("tcp", ip+":"+os.Getenv("SWITCH_TELNET_PORT"))
	if err != nil {
		return err
	}
	defer conn.Close()

	logger := io.MultiWriter(os.Stdout, file)

	conn.SetDeadline(time.Now().Add(timeout))

	err = login(conn, logger)
	if err != nil {
		return err
	}

	for _, cmd := range commands {
		if strings.TrimSpace(cmd) == "" {
			continue
		}

		fmt.Fprintf(logger, "\n[%s] Выполняем: %s\n", ip, cmd)

		conn.Write([]byte(cmd + "\n"))

		time.Sleep(time.Duration(commandDelay) * time.Millisecond)

		if err = readUntilPrompt(conn, ip, logger, timeout); err != nil {
			return err
		}
	}

	return nil
}

func readUntilPrompt(conn *telnet.Conn, ip string, logger io.Writer, timeout time.Duration) error {
	buffer := make([]byte, 1024)
	var output strings.Builder

	deadline := time.Now().Add(timeout)

	for {
		conn.SetReadDeadline(deadline)

		n, err := conn.Read(buffer)
		if err != nil {
			return err
		}

		chunk := string(buffer[:n])
		output.WriteString(chunk)

		if strings.Contains(output.String(), ">") ||
			strings.Contains(output.String(), ":") ||
			strings.Contains(output.String(), "?") ||
			strings.Contains(output.String(), "#") {

			fmt.Fprintf(logger, "[%s] Ответ: %s", ip, chunk)

			return nil
		}

		if strings.Contains(output.String(), "--More--") {
			conn.Write([]byte(" "))
			output.Reset()
		}
	}
}

func readLines(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func login(conn *telnet.Conn, logger io.Writer) error {
	buffer := make([]byte, 1024)
	var output strings.Builder

	timeout := time.Now().Add(15 * time.Second)

	userSent := false
	passSent := false

	for {
		conn.SetReadDeadline(timeout)

		n, err := conn.Read(buffer)
		if err != nil {
			return err
		}

		chunk := string(buffer[:n])
		output.WriteString(chunk)

		fmt.Fprint(logger, chunk)

		text := output.String()

		// --- username ---
		if !userSent &&
			(strings.Contains(text, "UserName:") ||
				strings.Contains(text, "Username:") ||
				strings.Contains(text, "login:")) {

			conn.Write([]byte(os.Getenv("SWITCH_USERNAME") + "\n"))
			userSent = true
			output.Reset()
			continue
		}

		// --- password ---
		if !passSent && strings.Contains(text, "Password:") {
			conn.Write([]byte(os.Getenv("SWITCH_PASSWORD") + "\n"))
			passSent = true
			output.Reset()
			continue
		}

		// --- успешный вход ---
		if strings.Contains(text, ">") ||
			strings.Contains(text, "#") {

			return nil
		}
	}
}
