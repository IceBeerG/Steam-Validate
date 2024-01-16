package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"
)

// var logSteam string

func main() {
	checkLog := "check.log"
	_, err := os.Stat(checkLog)
	if os.IsNotExist(err) {
		_, err := os.Create(checkLog)
		if err != nil {
			fmt.Println(err)
		}
	}
	var gameName, gameID string
	steamPath := regGet(`SOFTWARE\WOW6432Node\Valve\Steam`, "InstallPath") // получаем папку стима
	logSteam := steamPath + `\logs\content_log.txt`                        // лог для проверки окончания проверки игры
	steamConn := steamPath + `\logs\connection_log.txt`                    // лог подключения стима
	vdfSteam := steamPath + `\steamapps\libraryfolders.vdf`                // файл со списком библиотек стима
	vdfSteamUser := steamPath + `\config\loginusers.vdf`                   // файл с логином, проверка что стим авторизирован
	steamLibrary, err := parseSteamLibrary(vdfSteam)
	if err != nil {
		fmt.Println(err)
	}
	var login bool = false

	if !checkIfProcessRunning("steam.exe") && findString(vdfSteamUser, "1", "AllowAutoLogin") {
		fmt.Println("Запуск стима")
		runCommand("cmd", "/C", "start", "steam://run")
		time.Sleep(30 * time.Second)
	} else if !findString(vdfSteamUser, "1", "AllowAutoLogin") {
		for {
			if !checkIfProcessRunning("steamservice.exe") {
				err := os.Remove(steamConn)
				if err != nil {
					fmt.Printf("Ошибка удаления файла %s. %s", steamConn, err)
				}
				fmt.Println("Запуск стима. Авторизируйтесь в стиме")
				runCommand("cmd", "/C", "start", "steam://run")
				time.Sleep(10 * time.Second)
			} else if !checkLastString(steamConn, "] Logging on [") || !login {
				fmt.Println("Ждем авторизации в стиме")
				for {
					fmt.Print("*")
					if fileModify(steamConn) {
						if checkLastString(steamConn, "] Logging on [") {
							fmt.Println("\nСтим авторизован, проверка скоро начнется")
							fmt.Println("Если проверка не запустится, перезапустите приложение")
							login = true
							break
						}
					}
				}
			} else {
				break
			}
		}
	}

	time.Sleep(5 * time.Second)
	for _, path := range steamLibrary {
		manifest, err := filepath.Glob(path + `\appmanifest_*.acf`)
		if err != nil {
			fmt.Println(err)
		}
		for _, file_manifest := range manifest {
			fmt.Println()

			gameName, err = getInfo(file_manifest, "\"name\"")
			if err != nil {
				fmt.Println(err)
			}
			gameID, err = getInfo(file_manifest, "\"appid\"")
			if err != nil {
				fmt.Println(err)
			}
			gameDir := path + "\\" + gameName

			date := time.Now().Format("2006-01-02")
			dateM := date[:len(date)-3]
			checked := findString("check.log", dateM, gameID)

			if !checked {
				checkGames(gameID, gameDir, file_manifest, logSteam)
			} else {
				fmt.Println(gameName + " - Игра проверена ранее")
			}
		}
	}
	fmt.Println("\nПроверка игр завершена")
	g := ""
	fmt.Scan(&g)
}

// получаем данные из реестра
func regGet(regFolder, keys string) string {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, regFolder, registry.QUERY_VALUE)
	if err != nil {
		log.Printf("Ошибка открытия ветки реестра: %v. %s\n", err, getLine())
	}
	defer key.Close()

	value, _, err := key.GetStringValue(keys)
	if err != nil {
		log.Printf("Ошибка чтения папки стима: %v. %s\n", err, getLine())
	}
	return value
}

// получение строки кода где возникла ошибка
func getLine() string {
	_, _, line, _ := runtime.Caller(1)
	lineErr := fmt.Sprintf("\nСтрока: %d", line)
	return lineErr
}

func parseSteamLibrary(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)

	var data []string
	var currentPath string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, "\"path\"") {
			parts := strings.SplitN(line, "\"path\"", 2)
			path := strings.TrimSpace(parts[1])
			path = strings.Trim(path, "\"\t")
			path = strings.ReplaceAll(path, "\\\\", "\\")
			currentPath = path + "\\steamapps"
			data = append(data, currentPath)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Println("ошибка при сканировании файла:", err)
	}
	return data, err
}

func getInfo(fileName, trimString string) (string, error) {
	file, err := os.Open(fileName)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)

	var data string = ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, trimString) {
			parts := strings.SplitN(line, trimString, 2)
			stringManifest := strings.TrimSpace(parts[1])
			stringManifest = strings.Trim(stringManifest, "\"")
			data = stringManifest
		}
	}
	return data, err
}

func fileModify(filename string) (modify bool) {
	fileinfo, err := os.Stat(filename)
	if err != nil {
		fmt.Println("[ERROR] Ошибка получения инфо о файле", err)
	}
	prevModify := fileinfo.ModTime()
	for {
		time.Sleep(500 * time.Millisecond)
		fileinfo2, err := os.Stat(filename)
		if err != nil {
			fmt.Println("[ERROR] Ошибка получения инфо о файле", err)
		}
		if prevModify != fileinfo2.ModTime() {
			modify = true
			break
		}
	}
	return
}

func checkGames(id, game, file_manifest, logSteam string) {
	searchStrign1 := fmt.Sprintf("%s scheduler finished : removed from schedule", id)
	searchStrign2 := fmt.Sprintf("%s is marked \"NoUpdatesAfterInstall\"", id)

	validate := "steam://validate/" + id
	runCommand("cmd", "/C", "start", validate)
	fmt.Println("Запуск проверки игры ", game)

	for {
		if fileModify(logSteam) {
			if checkLastString(logSteam, searchStrign1) || checkLastString(logSteam, searchStrign2) {
				time.Sleep(1 * time.Second)

				fmt.Printf("Проверка %s завершена\n", game)

				checklog, err := os.OpenFile("check.log", os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					fmt.Println(err)
					break
				}
				defer checklog.Close()

				date := time.Now().Format("2006-01-02")
				checkedString := fmt.Sprintf("[%s] %s - Проверка завершена. %s\n", date, id, game)
				if _, err = checklog.WriteString(checkedString); err != nil {
					fmt.Println(err)
					break
				}
				break
			}
		}
	}
}

func runCommand(command string, args ...string) error {
	cmd := exec.Command(command, args...)
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func findString(filename, searchString, date string) bool {
	file, err := os.Open(filename) // "check.log"
	if err != nil {
		fmt.Println(err)
		// break
	}
	defer file.Close()

	var checked bool = false

	scanerF := bufio.NewScanner(file)
	for scanerF.Scan() {
		if date == "" {
			if strings.Contains(scanerF.Text(), searchString) {
				checked = true
				break
			}
		} else {
			if strings.Contains(scanerF.Text(), searchString) && strings.Contains(scanerF.Text(), date) {
				checked = true
				break
			}
		}
	}
	return checked
}

// Проверяет, запущен ли указанный процесс
func checkIfProcessRunning(processName string) bool {
	cmd := exec.Command("tasklist")
	output, err := cmd.Output()
	if err != nil {
		log.Println("[ERROR] Ошибка получения списка процессов:", err, getLine())
	}
	return strings.Contains(string(output), processName)
}

func checkLastString(filePath, searchString string) (checked bool) {
	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("Ошибка при открытии файла: %s", err)
	}
	defer file.Close()

	var lastLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lastLines = append(lastLines, scanner.Text())
		if len(lastLines) > 10 {
			lastLines = lastLines[1:]
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Ошибка при сканировании файла: %s", err)
	}

	for _, line := range lastLines {
		if strings.Contains(line, searchString) {
			checked = true
		}
	}
	return
}
