# 向 WebView2 子窗口投递真实鼠标消息（窗口内坐标 = CSS 像素，DPI=1）。
# 用在 cua / UIA 坐标点击不可用时（例如桌面处于锁屏安全桌面）驱动已打包的 EXE。
#
# 用法：
#   pwsh -File scripts/devtools/click-window-at.ps1 -X 508 -Y 141
#   pwsh -File scripts/devtools/click-window-at.ps1 -X 36 -Y 366 -DoubleClick

param(
    [Parameter(Mandatory = $true)][int]$X,
    [Parameter(Mandatory = $true)][int]$Y,
    [string]$ProcessNamePattern = 'LytVPK*',
    [string]$ChildClass = 'Chrome_WidgetWin_1',
    [int]$ClickCount = 1,
    [int]$DelayMs = 220,
    [switch]$DoubleClick
)

$ErrorActionPreference = 'Stop'

if (-not ('InputWin32' -as [type])) {
    Add-Type @'
using System;
using System.Runtime.InteropServices;
using System.Text;
public class InputWin32 {
    public delegate bool EnumProc(IntPtr hWnd, IntPtr lParam);
    [DllImport("user32.dll")] public static extern bool EnumChildWindows(IntPtr parent, EnumProc cb, IntPtr lParam);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] public static extern int GetClassNameW(IntPtr hWnd, StringBuilder text, int count);
    [DllImport("user32.dll")] public static extern bool PostMessageW(IntPtr hWnd, uint msg, IntPtr wParam, IntPtr lParam);
}
'@
}

$proc = Get-Process |
    Where-Object { $_.ProcessName -like $ProcessNamePattern -and $_.MainWindowHandle -ne 0 } |
    Select-Object -First 1
if (-not $proc) { throw "没有找到匹配 $ProcessNamePattern 的窗口" }

$target = $proc.MainWindowHandle
if ($ChildClass -ne '') {
    $script:foundChild = [IntPtr]::Zero
    $cb = [InputWin32+EnumProc]{
        param($hWnd, $lParam)
        $cls = New-Object System.Text.StringBuilder 256
        [InputWin32]::GetClassNameW($hWnd, $cls, 256) | Out-Null
        if ($cls.ToString() -eq $ChildClass) {
            $script:foundChild = $hWnd
            return $false
        }
        return $true
    }
    [InputWin32]::EnumChildWindows($target, $cb, [IntPtr]::Zero) | Out-Null
    if ($script:foundChild -eq [IntPtr]::Zero) { throw "没有找到 class=$ChildClass 的子窗口" }
    $target = $script:foundChild
}

$WM_MOUSEMOVE = 0x0200
$WM_LBUTTONDOWN = 0x0201
$WM_LBUTTONUP = 0x0202
$WM_LBUTTONDBLCLK = 0x0203

$lParam = [IntPtr](($Y -shl 16) -bor ($X -band 0xFFFF))

[InputWin32]::PostMessageW($target, $WM_MOUSEMOVE, [IntPtr]::Zero, $lParam) | Out-Null
Start-Sleep -Milliseconds 80

$clicks = if ($DoubleClick) { 2 } else { [Math]::Max(1, $ClickCount) }
for ($i = 0; $i -lt $clicks; $i++) {
    $downMsg = if ($DoubleClick -and $i -eq 1) { $WM_LBUTTONDBLCLK } else { $WM_LBUTTONDOWN }
    [InputWin32]::PostMessageW($target, $downMsg, [IntPtr]1, $lParam) | Out-Null
    Start-Sleep -Milliseconds 40
    [InputWin32]::PostMessageW($target, $WM_LBUTTONUP, [IntPtr]::Zero, $lParam) | Out-Null
    Start-Sleep -Milliseconds $DelayMs
}

"clicked $clicks time(s) at $X,$Y on hwnd $target"
