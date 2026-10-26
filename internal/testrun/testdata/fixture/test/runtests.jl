using Test

@testset "arithmetic" begin
    @test 1 + 1 == 2
    @test 2 * 2 == 5
    @test_skip 3 == 3
end

@testset "passing group" begin
    @test 4 == 4
end
