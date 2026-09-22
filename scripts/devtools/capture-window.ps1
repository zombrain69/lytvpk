# 用 PrintWindow 抓取指定窗口的真实像素（含 WebView2 内容）。
# 与普通截图不同：窗口被遮挡或桌面处于锁屏安全桌面时仍然可用。
#
# 用法：
#   pwsh -File scripts/devtools/capture-window.ps1 -OutFile shots\app.png
#   pwsh -File scripts/devtools/capture-window.ps1 -ChildClass Chrome_WidgetWin_1

param(
    [string]$OutFile = "",
    [string]$ProcessNamePattern = 'LytVPK*',
    [string]$ChildClass = ''
)

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing

$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
if ($OutFile -eq '') {
    $OutFile = Join-Path $repoRoot '.tmp-cua\shots\window.png'
}

if (-not ('PrintWin32' -as [type])) {
    Add-Type @'
using System;
using System.Runtime.InteropServices;
using System.Text;
public class PrintWin32 {
    [StructLayout(LayoutKind.Sequential)]
    public struct RECT { public int Left; public int Top; public int Right; public int Bottom; }
    [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr hWnd, out RECT lpRect);
    [DllImport("user32.dll", SetLastError = true)] public static extern bool PrintWindow(IntPtr hWnd, IntPtr hdcBlt, uint nFlags);
    public delegate bool EnumProc(IntPtr hWnd, IntPtr lParam);
    [DllImport("user32.dll")] public static extern bool EnumChildWindows(IntPtr parent, EnumProc cb, IntPtr lParam);
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] public static extern int GetClassNameW(IntPtr hWnd, StringBuilder text, int count);
}
'@
}

$outDir = Split-Path -Parent $OutFile
if ($outDir -and -not (Test-Path $outDir)) { New-Item -ItemType Directory -Path $outDir -Force | Out-Null }

$proc = Get-Process |
    Where-Object { $_.ProcessName -like $ProcessNamePattern -and $_.MainWindowHandle -ne 0 } |
    Select-Object -First 1
if (-not $proc) { throw "没有找到匹配 $ProcessNamePattern 的窗口" }

$handle = $proc.MainWindowHandle

if ($ChildClass -ne '') {
    $script:foundChild = [IntPtr]::Zero
    $cb = [PrintWin32+EnumProc]{
        param($hWnd, $lParam)
        $cls = New-Object System.Text.StringBuilder 256
        [PrintWin32]::GetClassNameW($hWnd, $cls, 256) | Out-Null
        if ($cls.ToString() -eq $ChildClass) {
            $script:foundChild = $hWnd
            return $false
        }
        return $true
    }
    [PrintWin32]::EnumChildWindows($handle, $cb, [IntPtr]::Zero) | Out-Null
    if ($script:foundChild -eq [IntPtr]::Zero) { throw "没有找到 class=$ChildClass 的子窗口" }
    $handle = $script:foundChild
}

$rect = New-Object PrintWin32+RECT
[PrintWin32]::GetWindowRect($handle, [ref]$rect) | Out-Null
$width = $rect.Right - $rect.Left
$height = $rect.Bottom - $rect.Top

$bitmap = New-Object System.Drawing.Bitmap($width, $height)
$graphics = [System.Drawing.Graphics]::FromImage($bitmap)
$hdc = $graphics.GetHdc()
$ok = [PrintWin32]::PrintWindow($handle, $hdc, 2)
$graphics.ReleaseHdc($hdc)
$graphics.Dispose()
$bitmap.Save($OutFile, [System.Drawing.Imaging.ImageFormat]::Png)
$bitmap.Dispose()

[pscustomobject]@{
    Process     = $proc.ProcessName
    Pid         = $proc.Id
    Hwnd        = $handle
    PrintWindow = $ok
    Size        = "$width x $height"
    File        = $OutFile
} | Format-List
