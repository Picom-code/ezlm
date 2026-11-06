import XCTest
@testable import EZLM

/// Small Macs (8 GB): which processes the wizard lists, and what it says when
/// nothing fits. The engine sends top_mem, not gpu_procs, on a Mac.
final class SmallMacTests: XCTestCase {
    private func mac(ramGB: UInt64, availGB: UInt64, gpu: [Proc]? = nil, top: [Proc]? = nil) -> Hardware {
        let g = UInt64(1 << 30)
        return Hardware(os: "darwin", arch: "arm64", cpuName: "Apple M1", cores: 8,
                        ramTotal: ramGB * g, ramAvail: availGB * g, diskFree: 200 * g,
                        gpus: [], backend: "metal", unified: true, gpuUsable: ramGB * g * 67 / 100,
                        gpuProcs: gpu, topMem: top)
    }

    private let safari = Proc(pid: 501, name: "Safari", mem: 1_400_000_000, killable: true)
    private let mail = Proc(pid: 502, name: "Mail", mem: 300_000_000, killable: true)

    func testMacListsBiggestMemoryUsers() {
        let hw = mac(ramGB: 8, availGB: 2, top: [safari, mail])
        XCTAssertEqual(MemoryUsers.rows(hw)?.map(\.name), ["Safari", "Mail"])
        XCTAssertEqual(MemoryUsers.title(hw), "Using the most memory right now")
        XCTAssertEqual(MemoryUsers.noFitHint(hw), "Quit an app on the right, then scan again.")
    }

    func testGraphicsCardListWinsWhenPresent() {
        let hw = mac(ramGB: 16, availGB: 6, gpu: [mail], top: [safari])
        XCTAssertEqual(MemoryUsers.rows(hw)?.map(\.name), ["Mail"])
        XCTAssertEqual(MemoryUsers.title(hw), "Using your graphics card")
    }

    func testNoListMeansNoPointerToTheRight() {
        let hw = mac(ramGB: 8, availGB: 2)
        XCTAssertNil(MemoryUsers.rows(hw))
        XCTAssertEqual(MemoryUsers.noFitHint(hw), "Close a few apps, then scan again.")
    }

    /// The engine's JSON for an 8 GB Mac with nothing to close and nothing fitting.
    func testDecodesMacHardwareWithTopMem() throws {
        let json = #"""
        {"os":"darwin","arch":"arm64","cpu_name":"Apple M1","cores":8,
         "ram_total":8589934592,"ram_avail":1610612736,"disk_free":107374182400,
         "backend":"metal","unified":true,"gpu_usable":5755258122,
         "top_mem":[{"pid":501,"name":"Safari","mem":1400000000,"killable":true}]}
        """#
        let d = JSONDecoder()
        d.keyDecodingStrategy = .convertFromSnakeCase
        let hw = try d.decode(Hardware.self, from: Data(json.utf8))
        XCTAssertEqual(MemoryUsers.rows(hw)?.first?.name, "Safari")
        XCTAssertEqual(hw.machineWord, "Mac")
    }
}
