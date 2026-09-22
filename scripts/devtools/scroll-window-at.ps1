# 向 WebView2 子窗口投递鼠标滚轮消息（用于自动化查看长页面）。
# 用法：pwsh -File scripts/devtools/scroll-window-at.ps1 -X 800 -Y 500 -Delta -600 -Times 6

param(
    [Parameter(Mandatory = $true)][int]$X,
    [Parameter(Mandatory = $true)][int]$Y,
    [int]$Delta = -600,
    [int]$Times = 1,
    [int]$DelayMs = 150,
    [string]$ProcessNamePattern = 'LytVPK*',
    [string]$ChildClass = 'Chrome_WidgetWin_1'
)

$ErrorActionPreference = 'Stop'

if (-not ('ScrollWin32' -as [type])) {
    Add-Type @'
using System;
using System.Runtime.InteropServices;
using System.Text;
public class ScrollWin32 {
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
    $cb = [ScrollWin32+EnumProc]{
        param($hWnd, $lParam)
        $cls = New-Object System.Text.StringBuilder 256
        [ScrollWin32]::GetClassNameW($hWnd, $cls, 256) | Out-Null
        if ($cls.ToString() -eq $ChildClass) {
            $script:foundChild = $hWnd
            return $false
        }
        return $true
    }
    [ScrollWin32]::EnumChildWindows($target, $cb, [IntPtr]::Zero) | Out-Null
    if ($script:foundChild -eq [IntPtr]::Zero) { throw "没有找到 class=$ChildClass 的子窗口" }
    $target = $script:foundChild
}

$WM_MOUSEWHEEL = 0x020A
$point = [IntPtr](($Y -shl 16) -bor ($X -band 0xFFFF))
$wheel = [IntPtr](($Delta -shl 16) -band 0xFFFF0000)

for ($i = 0; $i -lt [Math]::Max(1, $Times); $i++) {
    [ScrollWin32]::PostMessageW($target, $WM_MOUSEWHEEL, $wheel, $point) | Out-Null
    Start-Sleep -Milliseconds $DelayMs
}

"scrolled $Times x $Delta at $X,$Y"
