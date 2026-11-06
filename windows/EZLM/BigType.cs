namespace EZLM;

/// Big letters drawn with half blocks (5 dots tall, 3 text rows), for the
/// model's name at the top of an empty chat. Only what model names use:
/// letters, digits and a little punctuation.
static class BigType
{
    static readonly Dictionary<char, string> G = new()
    {
        ['A'] = ".##.|#..#|####|#..#|#..#", ['B'] = "###.|#..#|###.|#..#|###.", ['C'] = ".###|#...|#...|#...|.###",
        ['D'] = "###.|#..#|#..#|#..#|###.", ['E'] = "####|#...|###.|#...|####", ['F'] = "####|#...|###.|#...|#...",
        ['G'] = ".###|#...|#.##|#..#|.###", ['H'] = "#..#|#..#|####|#..#|#..#", ['I'] = "###|.#.|.#.|.#.|###",
        ['J'] = "...#|...#|...#|#..#|.##.", ['K'] = "#..#|#.#.|##..|#.#.|#..#", ['L'] = "#...|#...|#...|#...|####",
        ['M'] = "#...#|##.##|#.#.#|#...#|#...#", ['N'] = "#..#|##.#|#.##|#..#|#..#", ['O'] = ".##.|#..#|#..#|#..#|.##.",
        ['P'] = "###.|#..#|###.|#...|#...", ['Q'] = ".##.|#..#|#..#|#.#.|.#.#", ['R'] = "###.|#..#|###.|#.#.|#..#",
        ['S'] = ".###|#...|.##.|...#|###.", ['T'] = "###|.#.|.#.|.#.|.#.", ['U'] = "#..#|#..#|#..#|#..#|.##.",
        ['V'] = "#...#|#...#|.#.#.|.#.#.|..#..", ['W'] = "#...#|#...#|#.#.#|##.##|#...#", ['X'] = "#..#|#..#|.##.|#..#|#..#",
        ['Y'] = "#...#|.#.#.|..#..|..#..|..#..", ['Z'] = "####|...#|.##.|#...|####",
        ['0'] = ".##.|#.##|##.#|#..#|.##.", ['1'] = ".#.|##.|.#.|.#.|###", ['2'] = "###.|...#|.##.|#...|####",
        ['3'] = "###.|...#|.##.|...#|###.", ['4'] = "#..#|#..#|####|...#|...#", ['5'] = "####|#...|###.|...#|###.",
        ['6'] = ".##.|#...|###.|#..#|.##.", ['7'] = "####|...#|..#.|.#..|.#..", ['8'] = ".##.|#..#|.##.|#..#|.##.",
        ['9'] = ".##.|#..#|.###|...#|.##.", ['.'] = ".|.|.|.|#", ['-'] = "...|...|###|...|...", [' '] = "..|..|..|..|..",
        ['/'] = "..#|..#|.#.|#..|#..", [':'] = ".|#|.|#|.",
    };

    /// The words of a model name that fit in about max characters
    /// ("Nemotron 3.5 Bolt 74B A9B" becomes "Nemotron 3.5").
    public static string Fit(string name, int max = 14)
    {
        var words = name.Split(' ', StringSplitOptions.RemoveEmptyEntries);
        var outp = "";
        foreach (var w in words)
        {
            var next = outp.Length == 0 ? w : outp + " " + w;
            if (next.Length > max && outp.Length > 0) break;
            outp = next;
        }
        return outp.Length > max ? outp[..max] : outp;
    }

    /// Three lines of half blocks spelling text (unknown characters are skipped).
    public static string[] Lines(string text)
    {
        var glyphs = text.ToUpperInvariant().Where(G.ContainsKey).Select(c => G[c].Split('|')).ToList();
        var rows = Enumerable.Range(0, 6).Select(r => string.Concat(glyphs.Select(g => (r < 5 ? g[r] : new string('.', g[0].Length)) + "."))).ToArray();
        var outp = new string[3];
        for (int r = 0; r < 6; r += 2)
            outp[r / 2] = new string(rows[r].Select((c, i) => (c == '#', rows[r + 1][i] == '#') switch { (true, true) => '█', (true, false) => '▀', (false, true) => '▄', _ => ' ' }).ToArray());
        return outp;
    }
}
