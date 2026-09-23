# 向 WebView2 子窗口投递"按下 -> 多次移动 -> 抬起"的真实鼠标消息，
# 用来验证 CSS resize / 拖拽类交互（窗口内坐标 = CSS 像素，DPI=1）。
#
# 用法：
#   pwsh -File scripts/devtools/drag-window-at.ps1 -FromX 1284 -FromY 829 -ToX 1380 -ToY 890

param(
    [Parameter(Mandatory = $true)][int]$FromX,
    [Parameter(Mandatory = $true)][int]$FromY,
    [Parameter(Mandatory = $true)][int]$ToX,
    [Parameter(Mandatory = $true)][int]$ToY,
    [string]$ProcessNamePattern = 'LytVPK*',
    [string]$ChildClass = 'Chrome_WidgetWin_1',
    [int]$Steps = 12,
    [int]$StepDelayMs = 60
)

$ErrorActionPreference = 'Stop'

if (-not ('DragWin32' -as [type])) {
    Add-Type @'
using System;
using System.Runtime.InteropServices;
using System.Text;
public class DragWin32 {
    public delegate bool EnumProc(IntPtr hWnd, IntPtr lParam);
    [DllImport("user32.dll")] public static extern bool EnumChildWindows(IntPtr parent, EnumProc cb, IntPtr lParam);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] public static extern int GetClassNameW(IntPtr hWnd, StringBuilder text, int count);
    [DllImport("user32.dll")] public static extern bool PostMessageW(IntPtr hWnd, uint msg, IntPtr wParam, IntPtr lParam);
}
'@
}

$WM_MOUSEMOVE = 0x0200
$WM_LBUTTONDOWN = 0x0201
$WM_LBUTTONUP = 0x0202
$MK_LBUTTON = 0x0001

function New-LParam([int]$x, [int]$y) {
    return [IntPtr](($x -band 0xFFFF) -bor (($y -band 0xFFFF) -shl 16))
}

$proc = Get-Process |
    Where-Object { $_.ProcessName -like $ProcessNamePattern -and $_.MainWindowHandle -ne 0 } |
    Select-Object -First 1
if (-not $proc) { throw "没有找到匹配 $ProcessNamePattern 的窗口" }

$target = $proc.MainWindowHandle
if ($ChildClass -ne '') {
    $child = [IntPtr]::Zero
    $callback = [DragWin32+EnumProc] {
        param($hwnd, $lparam)
        $text = New-Object System.Text.StringBuilder 256
        [void][DragWin32]::GetClassNameW($hwnd, $text, $text.Capacity)
        if ($text.ToString() -eq $ChildClass) {
            $script:child = $hwnd
            return $false
        }
        return $true
    }
    [void][DragWin32]::EnumChildWindows($target, $callback, [IntPtr]::Zero)
    if ($script:child -ne [IntPtr]::Zero) { $target = $script:child }
}

[void][DragWin32]::PostMessageW($target, $WM_MOUSEMOVE, [IntPtr]::Zero, (New-LParam $FromX $FromY))
Start-Sleep -Milliseconds 80
[void][DragWin32]::PostMessageW($target, $WM_LBUTTONDOWN, [IntPtr]$MK_LBUTTON, (New-LParam $FromX $FromY))
Start-Sleep -Milliseconds $StepDelayMs

for ($i = 1; $i -le $Steps; $i++) {
    $x = [int]($FromX + (($ToX - $FromX) * $i / $Steps))
    $y = [int]($FromY + (($ToY - $FromY) * $i / $Steps))
    [void][DragWin32]::PostMessageW($target, $WM_MOUSEMOVE, [IntPtr]$MK_LBUTTON, (New-LParam $x $y))
    Start-Sleep -Milliseconds $StepDelayMs
}

[void][DragWin32]::PostMessageW($target, $WM_LBUTTONUP, [IntPtr]::Zero, (New-LParam $ToX $ToY))
Start-Sleep -Milliseconds 150
Write-Output "dragged from $FromX,$FromY to $ToX,$ToY on hwnd $target"
