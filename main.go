package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"github.com/ziutek/telnet"
)

const (
	timeout    = 5 * time.Second
	workerPool = 5
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatalln(err)
		return
	}

	ips := readLines("ips.txt")
	commands := readLines("commands.txt")

	jobs := make(chan string)
	var wg sync.WaitGroup

	for i := 0; i < workerPool; i++ {
		wg.Add(1)
		go worker(jobs, commands, &wg)
	}

	for _, ip := range ips {
		jobs <- ip
	}
	close(jobs)

	wg.Wait()
	fmt.Println("Готово.")
}

func worker(jobs <-chan string, commands []string, wg *sync.WaitGroup) {
	defer wg.Done()

	for ip := range jobs {
		fmt.Println("Подключаемся к", ip)
		err := handleSwitch(ip, commands)
		if err != nil {
			log.Printf("[%s] ошибка: %v\n", ip, err)
		} else {
			fmt.Printf("[%s] успешно\n", ip)
		}
	}
}

func handleSwitch(ip string, commands []string) error {
	conn, err := telnet.Dial("tcp", ip+":"+os.Getenv("SWITCH_TELNET_PORT"))
	if err != nil {
		return err
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(timeout))

	err = login(conn)
	if err != nil {
		return err
	}

	for _, cmd := range commands {
		if strings.TrimSpace(cmd) == "" {
			continue
		}

		fmt.Printf("[%s] Выполняем: %s\n", ip, cmd)

		conn.Write([]byte(cmd + "\n"))

		if err = readUntilPrompt(conn, timeout); err != nil {
			return err
		}
	}

	return nil
}

func readUntilPrompt(conn *telnet.Conn, timeout time.Duration) error {
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
		fmt.Print(chunk)
		output.WriteString(chunk)

		if strings.Contains(output.String(), ">") ||
			strings.Contains(output.String(), ":") ||
			strings.Contains(output.String(), "?") ||
			strings.Contains(output.String(), "#") {

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

func login(conn *telnet.Conn) error {
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
		fmt.Print(chunk) // debug
		output.WriteString(chunk)

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
