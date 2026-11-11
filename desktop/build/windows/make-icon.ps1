Add-Type -AssemblyName System.Drawing
$ErrorActionPreference = 'Stop'

$sizes = @(16, 24, 32, 48, 64, 128, 256)
$out = 'D:\code\shiguang\desktop\build\windows\icon.ico'

$master = New-Object System.Drawing.Bitmap 256, 256
$graphics = [System.Drawing.Graphics]::FromImage($master)
$graphics.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
$graphics.Clear([System.Drawing.Color]::Transparent)

$rounded = New-Object System.Drawing.Drawing2D.GraphicsPath
$r = 56
$rounded.AddArc(0, 0, $r, $r, 180, 90)
$rounded.AddArc(256 - $r, 0, $r, $r, 270, 90)
$rounded.AddArc(256 - $r, 256 - $r, $r, $r, 0, 90)
$rounded.AddArc(0, 256 - $r, $r, $r, 90, 90)
$rounded.CloseFigure()
$background = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::FromArgb(255, 31, 59, 46))
$graphics.FillPath($background, $rounded)

$leaf = New-Object System.Drawing.Drawing2D.GraphicsPath
$leaf.AddBezier(128, 44, 58, 78, 46, 156, 128, 212)
$leaf.AddBezier(128, 212, 210, 156, 198, 78, 128, 44)
$leaf.CloseFigure()
$cream = New-Object System.Drawing.SolidBrush ([System.Drawing.Color]::FromArgb(255, 245, 242, 235))
$graphics.FillPath($cream, $leaf)

$vein = New-Object System.Drawing.Pen ([System.Drawing.Color]::FromArgb(255, 31, 59, 46)), 6
$vein.StartCap = [System.Drawing.Drawing2D.LineCap]::Round
$vein.EndCap = [System.Drawing.Drawing2D.LineCap]::Round
$graphics.DrawBezier($vein, 128, 206, 128, 174, 128, 142, 128, 92)

$graphics.Dispose()

$images = New-Object System.Collections.Generic.List[byte[]]
foreach ($size in $sizes) {
    $scaled = New-Object System.Drawing.Bitmap $size, $size
    $g = [System.Drawing.Graphics]::FromImage($scaled)
    $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
    $g.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
    $g.DrawImage($master, 0, 0, $size, $size)
    $g.Dispose()
    $stream = New-Object System.IO.MemoryStream
    $scaled.Save($stream, [System.Drawing.Imaging.ImageFormat]::Png)
    $images.Add($stream.ToArray())
    $stream.Dispose()
    $scaled.Dispose()
}
$master.Dispose()

$file = [System.IO.File]::Create($out)
$writer = New-Object System.IO.BinaryWriter $file
$writer.Write([UInt16]0)
$writer.Write([UInt16]1)
$writer.Write([UInt16]$sizes.Count)
$offset = 6 + 16 * $sizes.Count
for ($i = 0; $i -lt $sizes.Count; $i++) {
    $size = $sizes[$i]
    $data = $images[$i]
    $writer.Write([Byte]$(if ($size -ge 256) { 0 } else { $size }))
    $writer.Write([Byte]$(if ($size -ge 256) { 0 } else { $size }))
    $writer.Write([Byte]0)
    $writer.Write([Byte]0)
    $writer.Write([UInt16]1)
    $writer.Write([UInt16]32)
    $writer.Write([UInt32]$data.Length)
    $writer.Write([UInt32]$offset)
    $offset += $data.Length
}
foreach ($data in $images) { $writer.Write($data) }
$writer.Dispose()
$file.Dispose()
Write-Output "wrote $out"
