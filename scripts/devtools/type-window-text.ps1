# 向 WebView2 子窗口投递 WM_CHAR，用来在已聚焦的输入框里输入文本
# （中文等非 ASCII 字符同样支持，因为 WM_CHAR 使用 UTF-16 码元）。
#
# 用法：
#   pwsh -File scripts/devtools/type-window-text.ps1 -Text "M16"
#   pwsh -File scripts/devtools/type-window-text.ps1 -Text "武士刀" -Clear

param(
    [Parameter(Mandatory = $true)][string]$Text,
    [string]$ProcessNamePattern = 'LytVPK*',
    [string]$ChildClass = 'Chrome_WidgetWin_1',
    [int]$DelayMs = 60,
    # 先发 Ctrl+A、Delete 清空输入框（可选）。
    [switch]$Clear
)

$ErrorActionPreference = 'Stop'

if (-not ('TypeWin32' -as [type])) {
    Add-Type @'
using System;
using System.Runtime.InteropServices;
using System.Text;
public class TypeWin32 {
    public delegate bool EnumProc(IntPtr hWnd, IntPtr lParam);
    [DllImport("user32.dll")] public static extern bool EnumChildWindows(IntPtr parent, EnumProc cb, IntPtr lParam);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] public static extern int GetClassNameW(IntPtr hWnd, StringBuilder text, int count);
    [DllImport("user32.dll")] public static extern bool PostMessageW(IntPtr hWnd, uint msg, IntPtr wParam, IntPtr lParam);
}
'@
}

$WM_KEYDOWN = 0x0100
$WM_KEYUP = 0x0101
$WM_CHAR = 0x0102
$VK_A = 0x41
$VK_DELETE = 0x2E

$proc = Get-Process |
    Where-Object { $_.ProcessName -like $ProcessNamePattern -and $_.MainWindowHandle -ne 0 } |
    Select-Object -First 1
if (-not $proc) { throw "没有找到匹配 $ProcessNamePattern 的窗口" }

$target = $proc.MainWindowHandle
if ($ChildClass -ne '') {
    $callback = [TypeWin32+EnumProc] {
        param($hwnd, $lparam)
        $name = New-Object System.Text.StringBuilder 256
        [void][TypeWin32]::GetClassNameW($hwnd, $name, $name.Capacity)
        if ($name.ToString() -eq $ChildClass) {
            $script:child = $hwnd
            return $false
        }
        return $true
    }
    [void][TypeWin32]::EnumChildWindows($target, $callback, [IntPtr]::Zero)
    if ($script:child -ne [IntPtr]::Zero) { $target = $script:child }
}

if ($Clear) {
    [void][TypeWin32]::PostMessageW($target, $WM_KEYDOWN, [IntPtr]$VK_A, [IntPtr]0x001E0001)
    [void][TypeWin32]::PostMessageW($target, $WM_KEYUP, [IntPtr]$VK_A, [IntPtr]0xC01E0001)
    Start-Sleep -Milliseconds 120
    [void][TypeWin32]::PostMessageW($target, $WM_KEYDOWN, [IntPtr]$VK_DELETE, [IntPtr]0x01530001)
    [void][TypeWin32]::PostMessageW($target, $WM_KEYUP, [IntPtr]$VK_DELETE, [IntPtr]0xC1530001)
    Start-Sleep -Milliseconds 120
}

foreach ($ch in $Text.ToCharArray()) {
    [void][TypeWin32]::PostMessageW($target, $WM_CHAR, [IntPtr][int]$ch, [IntPtr]1)
    Start-Sleep -Milliseconds $DelayMs
}

Write-Output "typed '$Text' on hwnd $target"
