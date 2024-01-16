package main

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"
)

var (
	allertCheck         bool = false
	changeStatusStation bool = true // спрятать станцию в начале проверки и показать по окончании проверки
)

func main() {
	if !checkIfProcessRunning("esme.exe") {
		changeStatusStation = false
		fmt.Println("Запущено не на станции, либо esme.exe не запущен и станция не видна")
	}

	if changeStatusStation {
		viewStation(false)
	}

	checkLog := "check.log"
	_, err := os.Stat(checkLog)
	if os.IsNotExist(err) {
		_, err := os.Create(checkLog)
		if err != nil {
			fmt.Println(err)
		}
	}
	checkLogAllert := "checkAllert.log"
	_, err = os.Stat(checkLogAllert)
	if os.IsNotExist(err) {
		_, err := os.Create(checkLogAllert)
		if err != nil {
			fmt.Println(err)
		}
	}
	var gameName, gameID string
	steamPath := regGet(`SOFTWARE\WOW6432Node\Valve\Steam`, "InstallPath") // получаем папку стима
	logSteam := steamPath + `\logs\content_log.txt`                        // лог для проверки окончания проверки игры
	logSteamOld := steamPath + `\logs\content_log.previous.txt`            // лог для проверки окончания проверки игры
	steamConn := steamPath + `\logs\connection_log.txt`                    // лог подключения стима
	vdfSteam := steamPath + `\steamapps\libraryfolders.vdf`                // файл со списком библиотек стима
	vdfSteamUser := steamPath + `\config\loginusers.vdf`                   // файл с логином, проверка что стим авторизирован
	steamStatLog := steamPath + `\logs\stats_log.txt`
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
			loginCheck, stringLogin := checkLastString(steamConn, "] Logging on [")
			if !checkIfProcessRunning("steamservice.exe") {
				fmt.Println("Запуск стима. Авторизируйтесь в стиме")
				runCommand("cmd", "/C", "start", "steam://run")
				time.Sleep(10 * time.Second)
			} else if !loginCheck || !login {
				fmt.Println("Ждем авторизации в стиме")
				for {
					fmt.Print("*")
					changeStat, _ := fileModify(steamConn)
					if changeStat {
						if loginCheck {
							fmt.Println(stringLogin)
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
		fmt.Println("Библиотека - ", path)
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

			date := time.Now().Format("02.01.2006")
			dateM := date[3:]
			checked := findString("check.log", dateM, gameID+" - Проверка ")

			if !checked {
				checkGames(gameID, gameName, file_manifest, logSteam, logSteamOld, steamStatLog)
			} else {
				fmt.Println(gameName + " - Игра проверена ранее")
			}
			time.Sleep(5 * time.Second)
		}
	}
	if allertCheck {
		fmt.Println("\nПроверка игр завершена c ошибками, подробнее в", checkLogAllert)
	} else {
		fmt.Println("\nПроверка игр завершена")
	}

	if changeStatusStation {
		viewStation(true)
	}

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

func fileModify(filename string) (modify, fileChange bool) {
	fileinfo, err := os.Stat(filename)
	if err != nil {
		fmt.Println("[ERROR] Ошибка получения инфо о файле", err)
	}
	prevModify := fileinfo.ModTime()
	prevSize := fileinfo.Size()
	for {
		time.Sleep(500 * time.Millisecond)
		fileinfo2, err := os.Stat(filename)
		if err != nil {
			fmt.Println("[ERROR] Ошибка получения инфо о файле", err)
			break
		}
		if prevModify != fileinfo2.ModTime() {
			modify = true
			if prevSize > fileinfo2.Size() {
				fileChange = true
			}
			break
		}
	}
	return
}

func checkGames(id, game, file_manifest, logSteam, logSteamOld, steamStatLog string) {
	searchStrign1 := fmt.Sprintf("%s scheduler finished : removed from schedule", id)
	searchStrign2 := fmt.Sprintf("%s is marked \"NoUpdatesAfterInstall\" - skipping validation", id)
	searchStrign3 := fmt.Sprintf(`%s] Loading stats from disk...failed to initialize KV from file!`, id)
	// searchStrign5 := fmt.Sprintf(`%s scheduler finished : removed from schedule (result No connection`, id)
	// searchStrign4 := fmt.Sprintf(`%s scheduler finished : removed from schedule (result Disk write failure`, id)
	// searchStrign6 := fmt.Sprintf("%s scheduler finished : removed from schedule (result Suspended", id)

	validate := "steam://validate/" + id
	runCommand("cmd", "/C", "start", validate)
	fmt.Println("Запуск проверки игры ", game)

	checklog, err := os.OpenFile("check.log", os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		fmt.Println(err)
	}
	defer checklog.Close()

	checkLogAllert, err := os.OpenFile("checkAllert.log", os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		fmt.Println(err)
	}
	defer checkLogAllert.Close()

	date := time.Now().Format("02.01.2006 15:04:05")
	startCheck := time.Now()
	checkedString := fmt.Sprintf("[%s] %s - Запуск проверки %s\n", date, id, game)
	if _, err := checklog.WriteString(checkedString); err != nil {
		fmt.Println(err)
	}

	for {
		newFile := false
		// fmt.Println("циклы проверки") // проверка
		changeStat, newFile := fileModify(logSteam)
		if changeStat {
			var finish3 bool = false
			var stringFinish3 string = ""
			finish1, stringFinish1 := checkLastString(logSteam, searchStrign1)
			if newFile {
				finish3, stringFinish3 = checkLastString(logSteamOld, searchStrign1)
			}
			finish2, _ := checkLastString(logSteam, searchStrign2)
			otherAccount, _ := checkLastString(steamStatLog, searchStrign3)
			if (finish1 || finish3) && !otherAccount {
				check1 := strings.Contains(stringFinish1, "No Error")
				check2 := strings.Contains(stringFinish3, "No Error")
				check3 := strings.Contains(stringFinish1, "Suspended")
				check4 := strings.Contains(stringFinish3, "Suspended")

				if check1 || check2 || check3 || check4 {
					date := time.Now().Format("02.01.2006 15:04:05")
					checkedString := fmt.Sprintf("[%s] %s - Проверка %s завершена\n", date, id, game)
					fmt.Println(checkedString)
					if _, err = checklog.WriteString(checkedString); err != nil {
						fmt.Println(err)
						break
					}
					time.Sleep(2 * time.Second)
					break
				} else if strings.Contains(stringFinish1, "result No connection") || strings.Contains(stringFinish3, "result No connection") {
					checkedString := fmt.Sprintf("[%s] %s - Отмена проверки %s. Нет связи.\n", date, id, game)
					fmt.Println(checkedString)
					allertCheck = true
					if _, err = checkLogAllert.WriteString(checkedString); err != nil {
						fmt.Println(err)
						break
					}
					time.Sleep(2 * time.Second)
					break
				} else if strings.Contains(stringFinish1, "result Disk write failure") || strings.Contains(stringFinish3, "result Disk write failure") {
					checkedString := fmt.Sprintf("[%s] %s - Повторите проверку %s. Ошибка записи.\n", date, id, game)
					fmt.Println(checkedString)
					allertCheck = true
					if _, err = checkLogAllert.WriteString(checkedString); err != nil {
						fmt.Println(err)
						break
					}
					time.Sleep(2 * time.Second)
					break
				}
			} else if otherAccount && (finish1 || finish3) {
				stopCheck := time.Now()
				duration := stopCheck.Sub(startCheck)
				minutes := int(duration.Seconds())
				// fmt.Println("startCheck -", startCheck, "stopCheck -", stopCheck, "разница -", minutes)
				if minutes < 90 {
					checkedString := fmt.Sprintf("[%s] %s - Игра %s установлена с другого аккаунта. Смените УЗ для проверки.\n", date, id, game)
					fmt.Println(checkedString)
					allertCheck = true
					if _, err = checkLogAllert.WriteString(checkedString); err != nil {
						fmt.Println(err)
						break
					}
				} else if minutes >= 90 {
					date := time.Now().Format("02.01.2006 15:04:05")
					checkedString := fmt.Sprintf("[%s] %s - Проверка %s завершена\n", date, id, game)
					fmt.Println(checkedString)
					if _, err = checklog.WriteString(checkedString); err != nil {
						fmt.Println(err)
						break
					}
				}
				time.Sleep(2 * time.Second)
				break
			} else if finish2 {
				checkedString := fmt.Sprintf("[%s] %s - проверка выполняется внутри игры %s.\n", date, id, game)
				fmt.Println(checkedString)
				allertCheck = true
				if _, err = checkLogAllert.WriteString(checkedString); err != nil {
					fmt.Println(err)
					break
				}
				time.Sleep(2 * time.Second)
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

func checkLastString(filePath, searchString string) (checked bool, finishString string) {
	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("Ошибка при открытии файла: %s", err)
	}
	defer file.Close()

	var lastLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lastLines = append(lastLines, scanner.Text())
		if len(lastLines) > 50 {
			lastLines = lastLines[1:]
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Ошибка при сканировании файла: %s", err)
	}

	for _, line := range lastLines {
		if strings.Contains(line, searchString) {
			checked = true
			finishString = line
			break
		}
	}
	return
}

// скрыть\отобразить станцию
func viewStation(seeSt bool) error {
	regFolder := `SOFTWARE\ITKey\Esme`
	serverID := regGet(regFolder, "last_server") // получаем ID сервера
	regFolder += `\servers\` + serverID
	authToken := regGet(regFolder, "auth_token") // получаем токен для авторизации

	resp, err := http.Get("https://services.drova.io")
	if err != nil {
		fmt.Println("Сайт недоступен")
	} else {
		if resp.StatusCode == http.StatusOK {
			var visible string
			if seeSt {
				visible = "true"
			} else {
				visible = "false"
			}

			url := "https://services.drova.io/server-manager/servers/" + serverID + "/set_published/" + visible

			request, err := http.NewRequest("POST", url, nil)
			if err != nil {
				fmt.Println("Ошибка при создании запроса:", err)
				return err
			}

			request.Header.Set("X-Auth-Token", authToken) // Установка заголовка X-Auth-Token
			client := &http.Client{}
			response, err := client.Do(request)
			if err != nil {
				fmt.Println("Ошибка при отправке запроса:", err)
				return err
			}
			defer response.Body.Close()
			if seeSt {
				fmt.Printf("Станция видна\n")
			} else {
				fmt.Printf("Станция скрыта\n")
			}
		}
	}
	return err
}
