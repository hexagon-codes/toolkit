#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <wchar.h>

// 原生载荷只验证 Win32 提交内存，不把 Go/race 的运行时开销计入测试预算。
#define ALLOCATION_BYTES ((SIZE_T)96 * 1024 * 1024)

static int publish_outcome(const wchar_t *path, const char *outcome) {
    wchar_t temporary[256];
    DWORD written = 0;
    size_t length = strlen(outcome);
    if (_snwprintf(temporary, 256, L"%ls.tmp", path) < 0) return 0;
    HANDLE file = CreateFileW(temporary, GENERIC_WRITE, 0, NULL, CREATE_ALWAYS,
                              FILE_ATTRIBUTE_NORMAL, NULL);
    if (file == INVALID_HANDLE_VALUE) return 0;
    BOOL ok = WriteFile(file, outcome, (DWORD)length, &written, NULL);
    BOOL closed = CloseHandle(file);
    if (!ok || !closed || written != length) return 0;
    return MoveFileExW(temporary, path, MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH) != 0;
}

static int child_payload(const wchar_t *path) {
    void *memory = VirtualAlloc(NULL, ALLOCATION_BYTES, MEM_COMMIT | MEM_RESERVE, PAGE_READWRITE);
    if (memory == NULL) {
        DWORD error = GetLastError();
        char outcome[64];
        if (error == ERROR_NOT_ENOUGH_MEMORY || error == ERROR_COMMITMENT_LIMIT) {
            snprintf(outcome, sizeof(outcome), "denied:%lu", (unsigned long)error);
        } else {
            snprintf(outcome, sizeof(outcome), "fixture-error:%lu", (unsigned long)error);
        }
        return publish_outcome(path, outcome) ? 0 : 65;
    }
    if (!publish_outcome(path, "ready")) {
        VirtualFree(memory, 0, MEM_RELEASE);
        return 66;
    }
    Sleep(30000);
    return VirtualFree(memory, 0, MEM_RELEASE) ? 0 : 67;
}

static int read_outcome(const wchar_t *path, char *outcome, DWORD capacity) {
    HANDLE file = CreateFileW(path, GENERIC_READ, FILE_SHARE_READ, NULL, OPEN_EXISTING,
                              FILE_ATTRIBUTE_NORMAL, NULL);
    if (file == INVALID_HANDLE_VALUE) return 0;
    DWORD length = 0;
    BOOL ok = ReadFile(file, outcome, capacity - 1, &length, NULL);
    BOOL closed = CloseHandle(file);
    if (!ok || !closed || length == 0) return 0;
    outcome[length] = '\0';
    return 1;
}

static int wait_outcome(HANDLE process, const wchar_t *path, char *outcome, DWORD capacity) {
    ULONGLONG deadline = GetTickCount64() + 5000;
    do {
        if (read_outcome(path, outcome, capacity)) return 1;
        DWORD state = WaitForSingleObject(process, 10);
        // 原子发布完成后子进程可能先退出，退出事件不能覆盖已经产生的结果。
        if (state == WAIT_OBJECT_0) return read_outcome(path, outcome, capacity);
        if (state != WAIT_TIMEOUT) return 0;
    } while (GetTickCount64() < deadline);
    return 0;
}

static int cleanup_children(PROCESS_INFORMATION *children, int count) {
    int ok = 1;
    for (int i = 0; i < count; ++i) {
        DWORD state = WaitForSingleObject(children[i].hProcess, 0);
        if (state == WAIT_TIMEOUT) {
            if (!TerminateProcess(children[i].hProcess, 1) &&
                WaitForSingleObject(children[i].hProcess, 0) != WAIT_OBJECT_0) ok = 0;
        } else if (state != WAIT_OBJECT_0) {
            ok = 0;
        }
        if (WaitForSingleObject(children[i].hProcess, 2000) != WAIT_OBJECT_0) ok = 0;
        if (!CloseHandle(children[i].hThread)) ok = 0;
        if (!CloseHandle(children[i].hProcess)) ok = 0;
    }
    return ok;
}

int wmain(int argc, wchar_t **argv) {
    if (argc == 3 && wcscmp(argv[1], L"child") == 0) return child_payload(argv[2]);
    if (argc != 3 || (wcscmp(argv[1], L"control") != 0 && wcscmp(argv[1], L"limit") != 0)) {
        printf("JOB_MEMORY_FIXTURE_FAILED:mode");
        return 0;
    }
    wchar_t *end = NULL;
    unsigned long long expected = wcstoull(argv[2], &end, 10);
    if (end == argv[2] || *end != L'\0') {
        printf("JOB_MEMORY_FIXTURE_FAILED:limit");
        return 0;
    }
    JOBOBJECT_EXTENDED_LIMIT_INFORMATION limits = {0};
    DWORD flags = JOB_OBJECT_LIMIT_PROCESS_MEMORY | JOB_OBJECT_LIMIT_JOB_MEMORY;
    int configured = 0;
    for (int attempt = 0; attempt < 20; ++attempt) {
        if (QueryInformationJobObject(NULL, JobObjectExtendedLimitInformation, &limits,
                                     sizeof(limits), NULL) &&
            (limits.BasicLimitInformation.LimitFlags & flags) == flags &&
            (unsigned long long)limits.ProcessMemoryLimit == expected &&
            (unsigned long long)limits.JobMemoryLimit == expected) {
            configured = 1;
            break;
        }
        Sleep(250);
    }
    if (!configured) {
        printf("JOB_MEMORY_FIXTURE_FAILED:job-limit");
        return 0;
    }
    wchar_t executable[32768];
    DWORD executable_length = GetModuleFileNameW(NULL, executable, 32768);
    if (executable_length == 0 || executable_length >= 32768) {
        printf("JOB_MEMORY_FIXTURE_FAILED:executable");
        return 0;
    }
    const wchar_t *paths[2] = {L"memory-child-0.result", L"memory-child-1.result"};
    PROCESS_INFORMATION children[2] = {{0}};
    char outcomes[2][64] = {{0}};
    char failure[128] = {0};
    int started = 0;
    for (int i = 0; i < 2; ++i) {
        wchar_t command[32768];
        if (_snwprintf(command, 32768, L"\"%ls\" child %ls", executable, paths[i]) < 0) {
            snprintf(failure, sizeof(failure), "JOB_MEMORY_FIXTURE_FAILED:child-%d-command", i);
            break;
        }
        STARTUPINFOW startup = {0};
        startup.cb = sizeof(startup);
        startup.dwFlags = STARTF_USESTDHANDLES;
        startup.hStdInput = GetStdHandle(STD_INPUT_HANDLE);
        startup.hStdOutput = GetStdHandle(STD_OUTPUT_HANDLE);
        startup.hStdError = GetStdHandle(STD_ERROR_HANDLE);
        if (!CreateProcessW(executable, command, NULL, NULL, TRUE, 0, NULL, NULL,
                            &startup, &children[i])) {
            snprintf(failure, sizeof(failure), "JOB_MEMORY_FIXTURE_FAILED:child-%d-start:%lu",
                     i, (unsigned long)GetLastError());
            break;
        }
        ++started;
        if (!wait_outcome(children[i].hProcess, paths[i], outcomes[i], sizeof(outcomes[i]))) {
            snprintf(failure, sizeof(failure), "JOB_MEMORY_FIXTURE_FAILED:child-%d-result", i);
            break;
        }
    }
    if (!cleanup_children(children, started)) {
        printf("JOB_MEMORY_FIXTURE_FAILED:cleanup");
        return 0;
    }
    if (failure[0] != '\0') {
        printf("%s", failure);
    } else if (wcscmp(argv[1], L"control") == 0) {
        if (strcmp(outcomes[0], "ready") == 0 && strcmp(outcomes[1], "ready") == 0) {
            printf("JOB_MEMORY_CONTROL_READY");
        } else {
            printf("JOB_MEMORY_FIXTURE_FAILED:control-outcomes:%s:%s", outcomes[0], outcomes[1]);
        }
    } else if (strcmp(outcomes[0], "ready") != 0) {
        printf("JOB_MEMORY_FIXTURE_FAILED:first-outcome:%s", outcomes[0]);
    } else if (strcmp(outcomes[1], "ready") == 0) {
        printf("JOB_MEMORY_LIMIT_NOT_ENFORCED");
    } else if (strcmp(outcomes[1], "denied:8") == 0 || strcmp(outcomes[1], "denied:1455") == 0) {
        printf("JOB_MEMORY_LIMIT_ENFORCED:%s", outcomes[1] + strlen("denied:"));
    } else {
        printf("JOB_MEMORY_FIXTURE_FAILED:second-outcome:%s", outcomes[1]);
    }
    return 0;
}
